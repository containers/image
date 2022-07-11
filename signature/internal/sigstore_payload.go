package internal

import (
	"bytes"
	"crypto"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/containers/image/v5/version"
	digest "github.com/opencontainers/go-digest"
	cosignSignature "github.com/sigstore/sigstore/pkg/signature"
)

const (
	cosignSignatureType         = "cosign container image signature"
	cosignHarcodedHashAlgorithm = crypto.SHA256
)

// UntrustedCosignPayload is a parsed content of a Cosign signature payload (not the full signature)
type UntrustedCosignPayload struct {
	UntrustedDockerManifestDigest digest.Digest
	UntrustedDockerReference      string // FIXME: more precise type?
	UntrustedCreatorID            *string
	// This is intentionally an int64; the native JSON float64 type would allow to represent _some_ sub-second precision,
	// but not nearly enough (with current timestamp values, a single unit in the last place is on the order of hundreds of nanoseconds).
	// So, this is explicitly an int64, and we reject fractional values. If we did need more precise timestamps eventually,
	// we would add another field, UntrustedTimestampNS int64.
	UntrustedTimestamp *int64
}

// NewUntrustedCosignPayload returns an untrustedCosignPayload object with
// the specified primary contents and appropriate metadata.
func NewUntrustedCosignPayload(dockerManifestDigest digest.Digest, dockerReference string) UntrustedCosignPayload {
	// Use intermediate variables for these values so that we can take their addresses.
	// Golang guarantees that they will have a new address on every execution.
	creatorID := "containers/image " + version.Version
	timestamp := time.Now().Unix()
	return UntrustedCosignPayload{
		UntrustedDockerManifestDigest: dockerManifestDigest,
		UntrustedDockerReference:      dockerReference,
		UntrustedCreatorID:            &creatorID,
		UntrustedTimestamp:            &timestamp,
	}
}

// Compile-time check that untrustedCosignPayload implements json.Marshaler
var _ json.Marshaler = (*UntrustedCosignPayload)(nil)

// MarshalJSON implements the json.Marshaler interface.
func (s UntrustedCosignPayload) MarshalJSON() ([]byte, error) {
	if s.UntrustedDockerManifestDigest == "" || s.UntrustedDockerReference == "" {
		return nil, errors.New("Unexpected empty signature content")
	}
	critical := map[string]interface{}{
		"type":     cosignSignatureType,
		"image":    map[string]string{"docker-manifest-digest": s.UntrustedDockerManifestDigest.String()},
		"identity": map[string]string{"docker-reference": s.UntrustedDockerReference},
	}
	optional := map[string]interface{}{}
	if s.UntrustedCreatorID != nil {
		optional["creator"] = *s.UntrustedCreatorID
	}
	if s.UntrustedTimestamp != nil {
		optional["timestamp"] = *s.UntrustedTimestamp
	}
	signature := map[string]interface{}{
		"critical": critical,
		"optional": optional,
	}
	return json.Marshal(signature)
}

// Compile-time check that untrustedCosignPayload implements json.Unmarshaler
var _ json.Unmarshaler = (*UntrustedCosignPayload)(nil)

// UnmarshalJSON implements the json.Unmarshaler interface
func (s *UntrustedCosignPayload) UnmarshalJSON(data []byte) error {
	err := s.strictUnmarshalJSON(data)
	if err != nil {
		if formatErr, ok := err.(JSONFormatError); ok {
			err = NewInvalidSignatureError(formatErr.Error())
		}
	}
	return err
}

// strictUnmarshalJSON is UnmarshalJSON, except that it may return the internal JSONFormatError error type.
// Splitting it into a separate function allows us to do the JSONFormatError → InvalidSignatureError in a single place, the caller.
func (s *UntrustedCosignPayload) strictUnmarshalJSON(data []byte) error {
	var critical, optional json.RawMessage
	if err := ParanoidUnmarshalJSONObjectExactFields(data, map[string]interface{}{
		"critical": &critical,
		"optional": &optional,
	}); err != nil {
		return err
	}

	var creatorID string
	var timestamp float64
	var gotCreatorID, gotTimestamp = false, false
	// Cosign generates "optional": null if there are no user-specified annotations.
	if !bytes.Equal(optional, []byte("null")) {
		if err := ParanoidUnmarshalJSONObject(optional, func(key string) interface{} {
			switch key {
			case "creator":
				gotCreatorID = true
				return &creatorID
			case "timestamp":
				gotTimestamp = true
				return &timestamp
			default:
				var ignore interface{}
				return &ignore
			}
		}); err != nil {
			return err
		}
	}
	if gotCreatorID {
		s.UntrustedCreatorID = &creatorID
	}
	if gotTimestamp {
		intTimestamp := int64(timestamp)
		if float64(intTimestamp) != timestamp {
			return NewInvalidSignatureError("Field optional.timestamp is not is not an integer")
		}
		s.UntrustedTimestamp = &intTimestamp
	}

	var t string
	var image, identity json.RawMessage
	if err := ParanoidUnmarshalJSONObjectExactFields(critical, map[string]interface{}{
		"type":     &t,
		"image":    &image,
		"identity": &identity,
	}); err != nil {
		return err
	}
	if t != cosignSignatureType {
		return NewInvalidSignatureError(fmt.Sprintf("Unrecognized signature type %s", t))
	}

	var digestString string
	if err := ParanoidUnmarshalJSONObjectExactFields(image, map[string]interface{}{
		"docker-manifest-digest": &digestString,
	}); err != nil {
		return err
	}
	s.UntrustedDockerManifestDigest = digest.Digest(digestString)

	return ParanoidUnmarshalJSONObjectExactFields(identity, map[string]interface{}{
		"docker-reference": &s.UntrustedDockerReference,
	})
}

// CosignPayloadAcceptanceRules specifies how to decide whether an untrusted payload is acceptable.
// We centralize the actual parsing and data extraction in VerifyCosignPayload; this supplies
// the policy.  We use an object instead of supplying func parameters to verifyAndExtractSignature
// because the functions have the same or similar types, so there is a risk of exchanging the functions;
// named members of this struct are more explicit.
type CosignPayloadAcceptanceRules struct {
	ValidateSignedDockerReference      func(string) error
	ValidateSignedDockerManifestDigest func(digest.Digest) error
}

// VerifyCosignPayload verifies unverifiedBase64Signature of unverifiedPayload was correctly created by publicKey, and that its principal components
// match expected values, both as specified by rules, and returns it.
// We return an *UntrustedCosignPayload, although nothing actually uses it,
// just to double-check against stupid typos.
func VerifyCosignPayload(publicKey crypto.PublicKey, unverifiedPayload []byte, unverifiedBase64Signature string, rules CosignPayloadAcceptanceRules) (*UntrustedCosignPayload, error) {
	verifier, err := cosignSignature.LoadVerifier(publicKey, cosignHarcodedHashAlgorithm)
	if err != nil {
		return nil, fmt.Errorf("creating verifier: %w", err)
	}

	unverifiedSignature, err := base64.StdEncoding.DecodeString(unverifiedBase64Signature)
	if err != nil {
		return nil, NewInvalidSignatureError(fmt.Sprintf("base64 decoding: %v", err))
	}
	// github.com/sigstore/cosign/pkg/cosign.verifyOCISignature uses signatureoptions.WithContext(),
	// which seems to be not used by anything. So we don’t bother.
	if err := verifier.VerifySignature(bytes.NewReader(unverifiedSignature), bytes.NewReader(unverifiedPayload)); err != nil {
		return nil, NewInvalidSignatureError(fmt.Sprintf("cryptographic signature verification failed: %v", err))
	}

	var unmatchedPayload UntrustedCosignPayload
	if err := json.Unmarshal(unverifiedPayload, &unmatchedPayload); err != nil {
		return nil, NewInvalidSignatureError(err.Error())
	}
	if err := rules.ValidateSignedDockerManifestDigest(unmatchedPayload.UntrustedDockerManifestDigest); err != nil {
		return nil, err
	}
	if err := rules.ValidateSignedDockerReference(unmatchedPayload.UntrustedDockerReference); err != nil {
		return nil, err
	}
	// CosignPayloadAcceptanceRules have accepted this value.
	return &unmatchedPayload, nil
}

// Note: Consider the API unstable until the code supports at least three different image formats or transports.

package signature

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/pkg/errors"

	"github.com/containers/image/version"
	"github.com/opencontainers/go-digest"
)

const (
	signatureType = "atomic container signature"
)

// InvalidSignatureError is returned when parsing an invalid signature.
type InvalidSignatureError struct {
	msg string
}

func (err InvalidSignatureError) Error() string {
	return err.msg
}

// Signature is a parsed content of a signature.
// The only way to get this structure from a blob should be as a return value from a successful call to verifyAndExtractSignature below.
type Signature struct {
	DockerManifestDigest digest.Digest
	DockerReference      string // FIXME: more precise type?
}

// untrustedSignature is a parsed content of a signature.
type untrustedSignature struct {
	UntrustedDockerManifestDigest digest.Digest
	UntrustedDockerReference      string // FIXME: more precise type?
}

// Compile-time check that untrustedSignature implements json.Marshaler
var _ json.Marshaler = (*untrustedSignature)(nil)

// MarshalJSON implements the json.Marshaler interface.
func (s untrustedSignature) MarshalJSON() ([]byte, error) {
	return s.marshalJSONWithVariables(time.Now().UTC().Unix(), "atomic "+version.Version)
}

// Implementation of MarshalJSON, with a caller-chosen values of the variable items to help testing.
func (s untrustedSignature) marshalJSONWithVariables(timestamp int64, creatorID string) ([]byte, error) {
	if s.UntrustedDockerManifestDigest == "" || s.UntrustedDockerReference == "" {
		return nil, errors.New("Unexpected empty signature content")
	}
	critical := map[string]interface{}{
		"type":     signatureType,
		"image":    map[string]string{"docker-manifest-digest": s.UntrustedDockerManifestDigest.String()},
		"identity": map[string]string{"docker-reference": s.UntrustedDockerReference},
	}
	optional := map[string]interface{}{
		"creator":   creatorID,
		"timestamp": timestamp,
	}
	signature := map[string]interface{}{
		"critical": critical,
		"optional": optional,
	}
	return json.Marshal(signature)
}

// Compile-time check that untrustedSignature implements json.Unmarshaler
var _ json.Unmarshaler = (*untrustedSignature)(nil)

// UnmarshalJSON implements the json.Unmarshaler interface
func (s *untrustedSignature) UnmarshalJSON(data []byte) error {
	err := s.strictUnmarshalJSON(data)
	if err != nil {
		if _, ok := err.(jsonFormatError); ok {
			err = InvalidSignatureError{msg: err.Error()}
		}
	}
	return err
}

// strictUnmarshalJSON is UnmarshalJSON, except that it may return the internal jsonFormatError error type.
// Splitting it into a separate function allows us to do the jsonFormatError → InvalidSignatureError in a single place, the caller.
func (s *untrustedSignature) strictUnmarshalJSON(data []byte) error {
	var untyped interface{}
	if err := json.Unmarshal(data, &untyped); err != nil {
		return err
	}
	o, ok := untyped.(map[string]interface{})
	if !ok {
		return InvalidSignatureError{msg: "Invalid signature format"}
	}
	if err := validateExactMapKeys(o, "critical", "optional"); err != nil {
		return err
	}

	c, err := mapField(o, "critical")
	if err != nil {
		return err
	}
	if err := validateExactMapKeys(c, "type", "image", "identity"); err != nil {
		return err
	}

	optional, err := mapField(o, "optional")
	if err != nil {
		return err
	}
	_ = optional // We don't use anything from here for now.

	t, err := stringField(c, "type")
	if err != nil {
		return err
	}
	if t != signatureType {
		return InvalidSignatureError{msg: fmt.Sprintf("Unrecognized signature type %s", t)}
	}

	image, err := mapField(c, "image")
	if err != nil {
		return err
	}
	if err := validateExactMapKeys(image, "docker-manifest-digest"); err != nil {
		return err
	}
	digestString, err := stringField(image, "docker-manifest-digest")
	if err != nil {
		return err
	}
	s.UntrustedDockerManifestDigest = digest.Digest(digestString)

	identity, err := mapField(c, "identity")
	if err != nil {
		return err
	}
	if err := validateExactMapKeys(identity, "docker-reference"); err != nil {
		return err
	}
	reference, err := stringField(identity, "docker-reference")
	if err != nil {
		return err
	}
	s.UntrustedDockerReference = reference

	return nil
}

// Sign formats the signature and returns a blob signed using mech and keyIdentity
// (If it seems surprising that this is a method on untrustedSignature, note that there
// isn’t a good reason to think that a key used by the user is trusted by any component
// of the system just because it is a private key — actually the presence of a private key
// on the system increases the likelihood of an a successful attack on that private key
// on that particular system.)
func (s untrustedSignature) sign(mech SigningMechanism, keyIdentity string) ([]byte, error) {
	json, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}

	return mech.Sign(json, keyIdentity)
}

// signatureAcceptanceRules specifies how to decide whether an untrusted signature is acceptable.
// We centralize the actual parsing and data extraction in verifyAndExtractSignature; this supplies
// the policy.  We use an object instead of supplying func parameters to verifyAndExtractSignature
// because the functions have the same or similar types, so there is a risk of exchanging the functions;
// named members of this struct are more explicit.
type signatureAcceptanceRules struct {
	validateKeyIdentity                func(string) error
	validateSignedDockerReference      func(string) error
	validateSignedDockerManifestDigest func(digest.Digest) error
}

// verifyAndExtractSignature verifies that unverifiedSignature has been signed, and that its principial components
// match expected values, both as specified by rules, and returns it
func verifyAndExtractSignature(mech SigningMechanism, unverifiedSignature []byte, rules signatureAcceptanceRules) (*Signature, error) {
	signed, keyIdentity, err := mech.Verify(unverifiedSignature)
	if err != nil {
		return nil, err
	}
	if err := rules.validateKeyIdentity(keyIdentity); err != nil {
		return nil, err
	}

	var unmatchedSignature untrustedSignature
	if err := json.Unmarshal(signed, &unmatchedSignature); err != nil {
		return nil, InvalidSignatureError{msg: err.Error()}
	}
	if err := rules.validateSignedDockerManifestDigest(unmatchedSignature.UntrustedDockerManifestDigest); err != nil {
		return nil, err
	}
	if err := rules.validateSignedDockerReference(unmatchedSignature.UntrustedDockerReference); err != nil {
		return nil, err
	}
	// signatureAcceptanceRules have accepted this value.
	return &Signature{
		DockerManifestDigest: unmatchedSignature.UntrustedDockerManifestDigest,
		DockerReference:      unmatchedSignature.UntrustedDockerReference,
	}, nil
}

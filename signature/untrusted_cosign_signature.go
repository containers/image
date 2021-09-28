package signature

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/opencontainers/go-digest"
)

const (
	cosignSignatureType = "cosign container image signature"
)

// untrustedCosignSignature is the payload format for sigstore signing an image using
// the simple signing format.
type untrustedCosignSignature struct {
	DockerManifestDigest digest.Digest
	DockerReference      string
}

// newUnstrustedCosignSignature returns a cosignSignature object with
// the specified primary contents and appropriate metadata.
func newUnstrustedCosignSignature(dockerManifestDigest digest.Digest, dockerReference string) untrustedCosignSignature {
	// Use intermediate variables for these values so that we can take their addresses.
	// Golang guarantees that they will have a new address on every execution.
	return untrustedCosignSignature{
		DockerManifestDigest: dockerManifestDigest,
		DockerReference:      dockerReference,
	}
}

// Compile-time check that cosignSignature implements json.Marshaler
var _ json.Marshaler = (*untrustedCosignSignature)(nil)

// MarshalJSON implements the json.Marshaler interface.
func (s untrustedCosignSignature) MarshalJSON() ([]byte, error) {
	if s.DockerManifestDigest == "" || s.DockerReference == "" {
		return nil, errors.New("Unexpected empty signature content")
	}
	critical := map[string]interface{}{
		"type":     cosignSignatureType,
		"image":    map[string]string{"docker-manifest-digest": s.DockerManifestDigest.String()},
		"identity": map[string]string{"docker-reference": s.DockerReference},
	}
	optional := map[string]interface{}{} // TODO: add annotations i.e. claims
	signature := map[string]interface{}{
		"critical": critical,
		"optional": optional,
	}
	return json.Marshal(signature)
}

// Compile-time check that cosignSignature implements json.Unmarshaler
var _ json.Unmarshaler = (*untrustedCosignSignature)(nil)

// UnmarshalJSON implements the json.Unmarshaler interface
func (s *untrustedCosignSignature) UnmarshalJSON(data []byte) error {
	err := s.strictUnmarshalJSON(data)
	if err != nil {
		if formatErr, ok := err.(jsonFormatError); ok {
			err = InvalidSignatureError{msg: formatErr.Error()}
		}
	}
	return err
}

// strictUnmarshalJSON is UnmarshalJSON, except that it may return the internal jsonFormatError error type.
// Splitting it into a separate function allows us to do the jsonFormatError → InvalidSignatureError in a single place, the caller.
func (s *untrustedCosignSignature) strictUnmarshalJSON(data []byte) error {
	var critical, optional json.RawMessage
	if err := paranoidUnmarshalJSONObjectExactFields(data, map[string]interface{}{
		"critical": &critical,
		"optional": &optional,
	}); err != nil {
		return err
	}

	var t string
	var image json.RawMessage
	if err := paranoidUnmarshalJSONObjectExactFields(critical, map[string]interface{}{
		"type":  &t,
		"image": &image,
	}); err != nil {
		return err
	}
	if t != cosignSignatureType {
		return InvalidSignatureError{msg: fmt.Sprintf("Unrecognized signature type %s", t)}
	}

	var digestString string
	if err := paranoidUnmarshalJSONObjectExactFields(image, map[string]interface{}{
		"docker-manifest-digest": &digestString,
	}); err != nil {
		return err
	}
	s.DockerManifestDigest = digest.Digest(digestString)

	return nil
}

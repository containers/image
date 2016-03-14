// Note: Consider the API unstable until the code supports at least three different image formats or transports.

package signature

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/projectatomic/skopeo"
)

const (
	signatureType      = "atomic container signature"
	signatureCreatorID = "atomic " + skopeo.Version
)

// InvalidSignatureError is returned when parsing an invalid signature.
type InvalidSignatureError struct {
	msg string
}

func (err InvalidSignatureError) Error() string {
	return err.msg
}

// Signature is a parsed content of a signature.
type Signature struct {
	DockerManifestDigest string // FIXME: more precise type?
	DockerReference      string // FIXME: more precise type?
}

// Wrap signature to add to it some methods which we don't want to make public.
type privateSignature struct {
	Signature
}

// MarshalJSON implements the json.Marshaler interface.
func (s privateSignature) MarshalJSON() ([]byte, error) {
	return s.marshalJSONWithVariables(time.Now().UTC().Unix(), signatureCreatorID)
}

// Implementation of MarshalJSON, with a caller-chosen values of the variable items to help testing.
func (s privateSignature) marshalJSONWithVariables(timestamp int64, creatorID string) ([]byte, error) {
	if s.DockerManifestDigest == "" || s.DockerReference == "" {
		return nil, errors.New("Unexpected empty signature content")
	}
	critical := map[string]interface{}{
		"type":     signatureType,
		"image":    map[string]string{"docker-manifest-digest": s.DockerManifestDigest},
		"identity": map[string]string{"docker-reference": s.DockerReference},
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

// validateExactMapKeys returns an error if the keys of m are not exactly expectedKeys, which must be pairwise distinct
func validateExactMapKeys(m map[string]interface{}, expectedKeys ...string) error {
	if len(m) != len(expectedKeys) {
		return InvalidSignatureError{msg: "Unexpected keys in a JSON object"}
	}

	for _, k := range expectedKeys {
		if _, ok := m[k]; !ok {
			return InvalidSignatureError{msg: fmt.Sprintf("Key %s missing in a JSON object", k)}
		}
	}
	// Assuming expectedKeys are pairwise distinct, we know m contains len(expectedKeys) different values in expectedKeys.
	return nil
}

// mapField returns a member fieldName of m, if it is a JSON map, or an error.
func mapField(m map[string]interface{}, fieldName string) (map[string]interface{}, error) {
	untyped, ok := m[fieldName]
	if !ok {
		return nil, InvalidSignatureError{msg: fmt.Sprintf("Field %s missing", fieldName)}
	}
	v, ok := untyped.(map[string]interface{})
	if !ok {
		return nil, InvalidSignatureError{msg: fmt.Sprintf("Field %s is not a JSON object", fieldName)}
	}
	return v, nil
}

// stringField returns a member fieldName of m, if it is a string, or an error.
func stringField(m map[string]interface{}, fieldName string) (string, error) {
	untyped, ok := m[fieldName]
	if !ok {
		return "", InvalidSignatureError{msg: fmt.Sprintf("Field %s missing", fieldName)}
	}
	v, ok := untyped.(string)
	if !ok {
		return "", InvalidSignatureError{msg: fmt.Sprintf("Field %s is not a JSON object", fieldName)}
	}
	return v, nil
}

// UnmarshalJSON implements the json.Unmarshaler interface
func (s *privateSignature) UnmarshalJSON(data []byte) error {
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
	digest, err := stringField(image, "docker-manifest-digest")
	if err != nil {
		return err
	}
	s.DockerManifestDigest = digest

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
	s.DockerReference = reference

	return nil
}

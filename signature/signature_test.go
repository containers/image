package signature

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvalidSignatureError(t *testing.T) {
	// A stupid test just to keep code coverage
	s := "test"
	err := InvalidSignatureError{msg: s}
	assert.Equal(t, s, err.Error())
}

func TestMarshalJSON(t *testing.T) {
	// Empty string values
	s := privateSignature{Signature{DockerManifestDigest: "", DockerReference: "_"}}
	_, err := s.MarshalJSON()
	assert.Error(t, err)
	s = privateSignature{Signature{DockerManifestDigest: "_", DockerReference: ""}}
	_, err = s.MarshalJSON()
	assert.Error(t, err)

	// Success
	s = privateSignature{Signature{DockerManifestDigest: "digest!@#", DockerReference: "reference#@!"}}
	marshaled, err := s.marshalJSONWithVariables(0, "CREATOR")
	require.NoError(t, err)
	assert.Equal(t, []byte("{\"critical\":{\"identity\":{\"docker-reference\":\"reference#@!\"},\"image\":{\"docker-manifest-digest\":\"digest!@#\"},\"type\":\"atomic container signature\"},\"optional\":{\"creator\":\"CREATOR\",\"timestamp\":0}}"),
		marshaled)

	// We can't test MarshalJSON directly because the timestamp will keep changing, so just test that
	// it doesn't fail. And call it through the JSON package for a good measure.
	_, err = json.Marshal(s)
	assert.NoError(t, err)
}

type mSI map[string]interface{} // To minimize typing the long name

func TestValidateExactMapKeys(t *testing.T) {
	// Empty map and keys
	err := validateExactMapKeys(mSI{})
	assert.NoError(t, err)

	// Success
	err = validateExactMapKeys(mSI{"a": nil, "b": 1}, "b", "a")
	assert.NoError(t, err)

	// Extra map keys
	err = validateExactMapKeys(mSI{"a": nil, "b": 1}, "a")
	assert.Error(t, err)

	// Extra expected keys
	err = validateExactMapKeys(mSI{"a": 1}, "b", "a")
	assert.Error(t, err)

	// Unexpected key values
	err = validateExactMapKeys(mSI{"a": 1}, "b")
	assert.Error(t, err)
}

func TestMapField(t *testing.T) {
	// Field not found
	_, err := mapField(mSI{"a": mSI{}}, "b")
	assert.Error(t, err)

	// Field has a wrong type
	_, err = mapField(mSI{"a": 1}, "a")
	assert.Error(t, err)

	// Success
	// FIXME? We can't use mSI as the type of child, that type apparently can't be converted to the raw map type.
	child := map[string]interface{}{"b": mSI{}}
	m, err := mapField(mSI{"a": child, "b": nil}, "a")
	require.NoError(t, err)
	assert.Equal(t, child, m)
}

func TestStringField(t *testing.T) {
	// Field not found
	_, err := stringField(mSI{"a": "x"}, "b")
	assert.Error(t, err)

	// Field has a wrong type
	_, err = stringField(mSI{"a": 1}, "a")
	assert.Error(t, err)

	// Success
	s, err := stringField(mSI{"a": "x", "b": nil}, "a")
	require.NoError(t, err)
	assert.Equal(t, "x", s)
}

// A short-hand way to get a JSON object field value or panic. No error handling done, we know
// what we are working with, a panic in a test is good enough, and fitting test cases on a single line
// is a priority.
func x(m mSI, fields ...string) mSI {
	for _, field := range fields {
		// Not .(mSI) because type assertion of an unnamed type to a named type always fails (the types
		// are not "identical"), but the assignment is fine because they are "assignable".
		m = m[field].(map[string]interface{})
	}
	return m
}

// Return the result of modifying validJSON with fn and unmarshaling it into *sig
func tryUnmarshalModified(t *testing.T, sig *privateSignature, validJSON []byte, modifyFn func(mSI)) error {
	var tmp mSI
	err := json.Unmarshal(validJSON, &tmp)
	require.NoError(t, err)

	modifyFn(tmp)

	testJSON, err := json.Marshal(tmp)
	require.NoError(t, err)

	return json.Unmarshal(testJSON, sig)
}

func TestUnmarshalJSON(t *testing.T) {
	var s privateSignature
	// Invalid input. Note that json.Unmarshal is guaranteed to validate input before calling our
	// UnmarshalJSON implementation; so test that first, then test our error handling for completeness.
	err := json.Unmarshal([]byte("&"), &s)
	assert.Error(t, err)
	err = s.UnmarshalJSON([]byte("&"))
	assert.Error(t, err)

	// Not an object
	err = json.Unmarshal([]byte("1"), &s)
	assert.Error(t, err)

	// Start with a valid JSON.
	validSig := privateSignature{
		Signature{
			DockerManifestDigest: "digest!@#",
			DockerReference:      "reference#@!",
		},
	}
	validJSON, err := validSig.MarshalJSON()
	require.NoError(t, err)

	// Success
	s = privateSignature{}
	err = json.Unmarshal(validJSON, &s)
	require.NoError(t, err)
	assert.Equal(t, validSig, s)

	// Various ways to corrupt the JSON
	breakFns := []func(mSI){
		// A top-level field is missing
		func(v mSI) { delete(v, "critical") },
		func(v mSI) { delete(v, "optional") },
		// Extra top-level sub-object
		func(v mSI) { v["unexpected"] = 1 },
		// "critical" not an object
		func(v mSI) { v["critical"] = 1 },
		// "optional" not an object
		func(v mSI) { v["optional"] = 1 },
		// A field of "critical" is missing
		func(v mSI) { delete(x(v, "critical"), "type") },
		func(v mSI) { delete(x(v, "critical"), "image") },
		func(v mSI) { delete(x(v, "critical"), "identity") },
		// Extra field of "critical"
		func(v mSI) { x(v, "critical")["unexpected"] = 1 },
		// Invalid "type"
		func(v mSI) { x(v, "critical")["type"] = 1 },
		func(v mSI) { x(v, "critical")["type"] = "unexpected" },
		// Invalid "image" object
		func(v mSI) { x(v, "critical")["image"] = 1 },
		func(v mSI) { delete(x(v, "critical", "image"), "docker-manifest-digest") },
		func(v mSI) { x(v, "critical", "image")["unexpected"] = 1 },
		// Invalid "docker-manifest-digest"
		func(v mSI) { x(v, "critical", "image")["docker-manifest-digest"] = 1 },
		// Invalid "identity" object
		func(v mSI) { x(v, "critical")["identity"] = 1 },
		func(v mSI) { delete(x(v, "critical", "identity"), "docker-reference") },
		func(v mSI) { x(v, "critical", "identity")["unexpected"] = 1 },
		// Invalid "docker-reference"
		func(v mSI) { x(v, "critical", "identity")["docker-reference"] = 1 },
	}
	for _, fn := range breakFns {
		err = tryUnmarshalModified(t, &s, validJSON, fn)
		assert.Error(t, err)
	}

	// Modifications to "optional" are allowed and ignored
	allowedModificationFns := []func(mSI){
		// Add an optional field
		func(v mSI) { x(v, "optional")["unexpected"] = 1 },
		// Delete an optional field
		func(v mSI) { delete(x(v, "optional"), "creator") },
	}
	for _, fn := range allowedModificationFns {
		s = privateSignature{}
		err = tryUnmarshalModified(t, &s, validJSON, fn)
		require.NoError(t, err)
		assert.Equal(t, validSig, s)
	}
}

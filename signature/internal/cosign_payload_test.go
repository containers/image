package internal

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/containers/image/v5/version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mSI map[string]interface{} // To minimize typing the long name

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

func TestNewUntrustedCosignPayload(t *testing.T) {
	timeBefore := time.Now()
	sig := NewUntrustedCosignPayload(TestImageManifestDigest, TestImageSignatureReference)
	assert.Equal(t, TestImageManifestDigest, sig.UntrustedDockerManifestDigest)
	assert.Equal(t, TestImageSignatureReference, sig.UntrustedDockerReference)
	require.NotNil(t, sig.UntrustedCreatorID)
	assert.Equal(t, "containers/image "+version.Version, *sig.UntrustedCreatorID)
	require.NotNil(t, sig.UntrustedTimestamp)
	timeAfter := time.Now()
	assert.True(t, timeBefore.Unix() <= *sig.UntrustedTimestamp)
	assert.True(t, *sig.UntrustedTimestamp <= timeAfter.Unix())
}

func TestUntrustedCosignPayloadMarshalJSON(t *testing.T) {
	// Empty string values
	s := NewUntrustedCosignPayload("", "_")
	_, err := s.MarshalJSON()
	assert.Error(t, err)
	s = NewUntrustedCosignPayload("_", "")
	_, err = s.MarshalJSON()
	assert.Error(t, err)

	// Success
	// Use intermediate variables for these values so that we can take their addresses.
	creatorID := "CREATOR"
	timestamp := int64(1484683104)
	for _, c := range []struct {
		input    UntrustedCosignPayload
		expected string
	}{
		{
			UntrustedCosignPayload{
				UntrustedDockerManifestDigest: "digest!@#",
				UntrustedDockerReference:      "reference#@!",
				UntrustedCreatorID:            &creatorID,
				UntrustedTimestamp:            &timestamp,
			},
			"{\"critical\":{\"identity\":{\"docker-reference\":\"reference#@!\"},\"image\":{\"docker-manifest-digest\":\"digest!@#\"},\"type\":\"cosign container image signature\"},\"optional\":{\"creator\":\"CREATOR\",\"timestamp\":1484683104}}",
		},
		{
			UntrustedCosignPayload{
				UntrustedDockerManifestDigest: "digest!@#",
				UntrustedDockerReference:      "reference#@!",
			},
			"{\"critical\":{\"identity\":{\"docker-reference\":\"reference#@!\"},\"image\":{\"docker-manifest-digest\":\"digest!@#\"},\"type\":\"cosign container image signature\"},\"optional\":{}}",
		},
	} {
		marshaled, err := c.input.MarshalJSON()
		require.NoError(t, err)
		assert.Equal(t, []byte(c.expected), marshaled)

		// Also call MarshalJSON through the JSON package.
		marshaled, err = json.Marshal(c.input)
		assert.NoError(t, err)
		assert.Equal(t, []byte(c.expected), marshaled)
	}
}

// Return the result of modifying validJSON with fn
func modifiedUntrustedCosignPayloadJSON(t *testing.T, validJSON []byte, modifyFn func(mSI)) []byte {
	var tmp mSI
	err := json.Unmarshal(validJSON, &tmp)
	require.NoError(t, err)

	modifyFn(tmp)

	modifiedJSON, err := json.Marshal(tmp)
	require.NoError(t, err)
	return modifiedJSON
}

// Verify that input can be unmarshaled as an untrustedCosignPayload.
func successfullyUnmarshalUntrustedCosignPayload(t *testing.T, input []byte) UntrustedCosignPayload {
	var s UntrustedCosignPayload
	err := json.Unmarshal(input, &s)
	require.NoError(t, err, string(input))

	return s
}

// Verify that input can't be unmarshaled as an untrustedCosignPayload.
func assertUnmarshalUntrustedCosignPayloadFails(t *testing.T, input []byte) {
	var s UntrustedCosignPayload
	err := json.Unmarshal(input, &s)
	assert.Error(t, err, string(input))
}

func TestUntrustedCosignPayloadUnmarshalJSON(t *testing.T) {
	// Invalid input. Note that json.Unmarshal is guaranteed to validate input before calling our
	// UnmarshalJSON implementation; so test that first, then test our error handling for completeness.
	assertUnmarshalUntrustedCosignPayloadFails(t, []byte("&"))
	var s UntrustedCosignPayload
	err := s.UnmarshalJSON([]byte("&"))
	assert.Error(t, err)

	// Not an object
	assertUnmarshalUntrustedCosignPayloadFails(t, []byte("1"))

	// Start with a valid JSON.
	validSig := NewUntrustedCosignPayload("digest!@#", "reference#@!")
	validJSON, err := validSig.MarshalJSON()
	require.NoError(t, err)

	// Success
	s = successfullyUnmarshalUntrustedCosignPayload(t, validJSON)
	assert.Equal(t, validSig, s)

	// A Cosign-generated payload is handled correctly
	s = successfullyUnmarshalUntrustedCosignPayload(t, []byte(`{"critical":{"identity":{"docker-reference":"192.168.64.2:5000/cosign-signed-multi"},"image":{"docker-manifest-digest":"sha256:43955d6857268cc948ae9b370b221091057de83c4962da0826f9a2bdc9bd6b44"},"type":"cosign container image signature"},"optional":null}`))
	assert.Equal(t, UntrustedCosignPayload{
		UntrustedDockerManifestDigest: "sha256:43955d6857268cc948ae9b370b221091057de83c4962da0826f9a2bdc9bd6b44",
		UntrustedDockerReference:      "192.168.64.2:5000/cosign-signed-multi",
		UntrustedCreatorID:            nil,
		UntrustedTimestamp:            nil,
	}, s)

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
		// Invalid "creator"
		func(v mSI) { x(v, "optional")["creator"] = 1 },
		// Invalid "timestamp"
		func(v mSI) { x(v, "optional")["timestamp"] = "unexpected" },
		func(v mSI) { x(v, "optional")["timestamp"] = 0.5 }, // Fractional input
	}
	for _, fn := range breakFns {
		testJSON := modifiedUntrustedCosignPayloadJSON(t, validJSON, fn)
		assertUnmarshalUntrustedCosignPayloadFails(t, testJSON)
	}

	// Modifications to unrecognized fields in "optional" are allowed and ignored
	allowedModificationFns := []func(mSI){
		// Add an optional field
		func(v mSI) { x(v, "optional")["unexpected"] = 1 },
	}
	for _, fn := range allowedModificationFns {
		testJSON := modifiedUntrustedCosignPayloadJSON(t, validJSON, fn)
		s := successfullyUnmarshalUntrustedCosignPayload(t, testJSON)
		assert.Equal(t, validSig, s)
	}

	// Optional fields can be missing
	validSig = UntrustedCosignPayload{
		UntrustedDockerManifestDigest: "digest!@#",
		UntrustedDockerReference:      "reference#@!",
		UntrustedCreatorID:            nil,
		UntrustedTimestamp:            nil,
	}
	validJSON, err = validSig.MarshalJSON()
	require.NoError(t, err)
	s = successfullyUnmarshalUntrustedCosignPayload(t, validJSON)
	assert.Equal(t, validSig, s)
}

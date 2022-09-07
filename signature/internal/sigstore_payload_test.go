package internal

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/containers/image/v5/internal/signature"
	"github.com/containers/image/v5/version"
	digest "github.com/opencontainers/go-digest"
	"github.com/sigstore/sigstore/pkg/cryptoutils"
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

func TestNewUntrustedSigstorePayload(t *testing.T) {
	timeBefore := time.Now()
	sig := NewUntrustedSigstorePayload(TestImageManifestDigest, TestImageSignatureReference)
	assert.Equal(t, TestImageManifestDigest, sig.UntrustedDockerManifestDigest)
	assert.Equal(t, TestImageSignatureReference, sig.UntrustedDockerReference)
	require.NotNil(t, sig.UntrustedCreatorID)
	assert.Equal(t, "containers/image "+version.Version, *sig.UntrustedCreatorID)
	require.NotNil(t, sig.UntrustedTimestamp)
	timeAfter := time.Now()
	assert.True(t, timeBefore.Unix() <= *sig.UntrustedTimestamp)
	assert.True(t, *sig.UntrustedTimestamp <= timeAfter.Unix())
}

func TestUntrustedSigstorePayloadMarshalJSON(t *testing.T) {
	// Empty string values
	s := NewUntrustedSigstorePayload("", "_")
	_, err := s.MarshalJSON()
	assert.Error(t, err)
	s = NewUntrustedSigstorePayload("_", "")
	_, err = s.MarshalJSON()
	assert.Error(t, err)

	// Success
	// Use intermediate variables for these values so that we can take their addresses.
	creatorID := "CREATOR"
	timestamp := int64(1484683104)
	for _, c := range []struct {
		input    UntrustedSigstorePayload
		expected string
	}{
		{
			UntrustedSigstorePayload{
				UntrustedDockerManifestDigest: "digest!@#",
				UntrustedDockerReference:      "reference#@!",
				UntrustedCreatorID:            &creatorID,
				UntrustedTimestamp:            &timestamp,
			},
			"{\"critical\":{\"identity\":{\"docker-reference\":\"reference#@!\"},\"image\":{\"docker-manifest-digest\":\"digest!@#\"},\"type\":\"cosign container image signature\"},\"optional\":{\"creator\":\"CREATOR\",\"timestamp\":1484683104}}",
		},
		{
			UntrustedSigstorePayload{
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
func modifiedUntrustedSigstorePayloadJSON(t *testing.T, validJSON []byte, modifyFn func(mSI)) []byte {
	var tmp mSI
	err := json.Unmarshal(validJSON, &tmp)
	require.NoError(t, err)

	modifyFn(tmp)

	modifiedJSON, err := json.Marshal(tmp)
	require.NoError(t, err)
	return modifiedJSON
}

// Verify that input can be unmarshaled as an UntrustedSigstorePayload.
func successfullyUnmarshalUntrustedSigstorePayload(t *testing.T, input []byte) UntrustedSigstorePayload {
	var s UntrustedSigstorePayload
	err := json.Unmarshal(input, &s)
	require.NoError(t, err, string(input))

	return s
}

// Verify that input can't be unmarshaled as an UntrustedSigstorePayload.
func assertUnmarshalUntrustedSigstorePayloadFails(t *testing.T, input []byte) {
	var s UntrustedSigstorePayload
	err := json.Unmarshal(input, &s)
	assert.Error(t, err, string(input))
}

func TestUntrustedSigstorePayloadUnmarshalJSON(t *testing.T) {
	// Invalid input. Note that json.Unmarshal is guaranteed to validate input before calling our
	// UnmarshalJSON implementation; so test that first, then test our error handling for completeness.
	assertUnmarshalUntrustedSigstorePayloadFails(t, []byte("&"))
	var s UntrustedSigstorePayload
	err := s.UnmarshalJSON([]byte("&"))
	assert.Error(t, err)

	// Not an object
	assertUnmarshalUntrustedSigstorePayloadFails(t, []byte("1"))

	// Start with a valid JSON.
	validSig := NewUntrustedSigstorePayload("digest!@#", "reference#@!")
	validJSON, err := validSig.MarshalJSON()
	require.NoError(t, err)

	// Success
	s = successfullyUnmarshalUntrustedSigstorePayload(t, validJSON)
	assert.Equal(t, validSig, s)

	// A /usr/bin/cosign-generated payload is handled correctly
	s = successfullyUnmarshalUntrustedSigstorePayload(t, []byte(`{"critical":{"identity":{"docker-reference":"192.168.64.2:5000/cosign-signed-multi"},"image":{"docker-manifest-digest":"sha256:43955d6857268cc948ae9b370b221091057de83c4962da0826f9a2bdc9bd6b44"},"type":"cosign container image signature"},"optional":null}`))
	assert.Equal(t, UntrustedSigstorePayload{
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
		testJSON := modifiedUntrustedSigstorePayloadJSON(t, validJSON, fn)
		assertUnmarshalUntrustedSigstorePayloadFails(t, testJSON)
	}

	// Modifications to unrecognized fields in "optional" are allowed and ignored
	allowedModificationFns := []func(mSI){
		// Add an optional field
		func(v mSI) { x(v, "optional")["unexpected"] = 1 },
	}
	for _, fn := range allowedModificationFns {
		testJSON := modifiedUntrustedSigstorePayloadJSON(t, validJSON, fn)
		s := successfullyUnmarshalUntrustedSigstorePayload(t, testJSON)
		assert.Equal(t, validSig, s)
	}

	// Optional fields can be missing
	validSig = UntrustedSigstorePayload{
		UntrustedDockerManifestDigest: "digest!@#",
		UntrustedDockerReference:      "reference#@!",
		UntrustedCreatorID:            nil,
		UntrustedTimestamp:            nil,
	}
	validJSON, err = validSig.MarshalJSON()
	require.NoError(t, err)
	s = successfullyUnmarshalUntrustedSigstorePayload(t, validJSON)
	assert.Equal(t, validSig, s)
}

func TestVerifySigstorePayload(t *testing.T) {
	publicKeyPEM, err := os.ReadFile("./testdata/cosign.pub")
	require.NoError(t, err)
	publicKey, err := cryptoutils.UnmarshalPEMToPublicKey(publicKeyPEM)
	require.NoError(t, err)

	type acceptanceData struct {
		signedDockerReference      string
		signedDockerManifestDigest digest.Digest
	}
	var wanted, recorded acceptanceData
	// recordingRules are a plausible SigstorePayloadAcceptanceRules implementations, but equally
	// importantly record that we are passing the correct values to the rule callbacks.
	recordingRules := SigstorePayloadAcceptanceRules{
		ValidateSignedDockerReference: func(signedDockerReference string) error {
			recorded.signedDockerReference = signedDockerReference
			if signedDockerReference != wanted.signedDockerReference {
				return errors.New("signedDockerReference mismatch")
			}
			return nil
		},
		ValidateSignedDockerManifestDigest: func(signedDockerManifestDigest digest.Digest) error {
			recorded.signedDockerManifestDigest = signedDockerManifestDigest
			if signedDockerManifestDigest != wanted.signedDockerManifestDigest {
				return errors.New("signedDockerManifestDigest mismatch")
			}
			return nil
		},
	}

	sigBlob, err := os.ReadFile("./testdata/valid.signature")
	require.NoError(t, err)
	genericSig, err := signature.FromBlob(sigBlob)
	require.NoError(t, err)
	sigstoreSig, ok := genericSig.(signature.Sigstore)
	require.True(t, ok)
	cryptoBase64Sig, ok := sigstoreSig.UntrustedAnnotations()[signature.SigstoreSignatureAnnotationKey]
	require.True(t, ok)
	signatureData := acceptanceData{
		signedDockerReference:      TestSigstoreSignatureReference,
		signedDockerManifestDigest: TestSigstoreManifestDigest,
	}

	// Successful verification
	wanted = signatureData
	recorded = acceptanceData{}
	res, err := VerifySigstorePayload(publicKey, sigstoreSig.UntrustedPayload(), cryptoBase64Sig, recordingRules)
	require.NoError(t, err)
	assert.Equal(t, res, &UntrustedSigstorePayload{
		UntrustedDockerManifestDigest: TestSigstoreManifestDigest,
		UntrustedDockerReference:      TestSigstoreSignatureReference,
		UntrustedCreatorID:            nil,
		UntrustedTimestamp:            nil,
	})
	assert.Equal(t, signatureData, recorded)

	// For extra paranoia, test that we return a nil signature object on error.

	// Invalid verifier
	recorded = acceptanceData{}
	invalidPublicKey := struct{}{} // crypto.PublicKey is, for some reason, just an interface{}, so this is acceptable.
	res, err = VerifySigstorePayload(invalidPublicKey, sigstoreSig.UntrustedPayload(), cryptoBase64Sig, recordingRules)
	assert.Error(t, err)
	assert.Nil(t, res)
	assert.Equal(t, acceptanceData{}, recorded)

	// Invalid base64 encoding
	for _, invalidBase64Sig := range []string{
		"&",                                      // Invalid base64 characters
		cryptoBase64Sig + "=",                    // Extra padding
		cryptoBase64Sig[:len(cryptoBase64Sig)-1], // Truncated base64 data
	} {
		recorded = acceptanceData{}
		res, err = VerifySigstorePayload(publicKey, sigstoreSig.UntrustedPayload(), invalidBase64Sig, recordingRules)
		assert.Error(t, err)
		assert.Nil(t, res)
		assert.Equal(t, acceptanceData{}, recorded)
	}

	// Invalid signature
	validSignatureBytes, err := base64.StdEncoding.DecodeString(cryptoBase64Sig)
	require.NoError(t, err)
	for _, invalidSig := range [][]byte{
		{}, // Empty signature
		[]byte("invalid signature"),
		append(validSignatureBytes, validSignatureBytes...),
	} {
		recorded = acceptanceData{}
		res, err = VerifySigstorePayload(publicKey, sigstoreSig.UntrustedPayload(), base64.StdEncoding.EncodeToString(invalidSig), recordingRules)
		assert.Error(t, err)
		assert.Nil(t, res)
		assert.Equal(t, acceptanceData{}, recorded)
	}

	// Valid signature of non-JSON
	recorded = acceptanceData{}
	res, err = VerifySigstorePayload(publicKey, []byte("&"), "MEUCIARnnxZQPALBfqkB4aNAYXad79Qs6VehcrgIeZ8p7I2FAiEAzq2HXwXlz1iJeh+ucUR3L0zpjynQk6Rk0+/gXYp49RU=", recordingRules)
	assert.Error(t, err)
	assert.Nil(t, res)
	assert.Equal(t, acceptanceData{}, recorded)

	// Valid signature of an unacceptable JSON
	recorded = acceptanceData{}
	res, err = VerifySigstorePayload(publicKey, []byte("{}"), "MEUCIQDkySOBGxastVP0+koTA33NH5hXjwosFau4rxTPN6g48QIgb7eWKkGqfEpHMM3aT4xiqyP/170jEkdFuciuwN4mux4=", recordingRules)
	assert.Error(t, err)
	assert.Nil(t, res)
	assert.Equal(t, acceptanceData{}, recorded)

	// Valid signature with a wrong manifest digest: asked for signedDockerManifestDigest
	wanted = signatureData
	wanted.signedDockerManifestDigest = "invalid digest"
	recorded = acceptanceData{}
	res, err = VerifySigstorePayload(publicKey, sigstoreSig.UntrustedPayload(), cryptoBase64Sig, recordingRules)
	assert.Error(t, err)
	assert.Nil(t, res)
	assert.Equal(t, acceptanceData{
		signedDockerManifestDigest: signatureData.signedDockerManifestDigest,
	}, recorded)

	// Valid signature with a wrong image reference
	wanted = signatureData
	wanted.signedDockerReference = "unexpected docker reference"
	recorded = acceptanceData{}
	res, err = VerifySigstorePayload(publicKey, sigstoreSig.UntrustedPayload(), cryptoBase64Sig, recordingRules)
	assert.Error(t, err)
	assert.Nil(t, res)
	assert.Equal(t, signatureData, recorded)
}

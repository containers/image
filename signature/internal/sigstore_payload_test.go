package internal

import (
	"bytes"
	"crypto"
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

// A short-hand way to get a JSON object field value or panic. No error handling done, we know
// what we are working with, a panic in a test is good enough, and fitting test cases on a single line
// is a priority.
func x(m mSA, fields ...string) mSA {
	for _, field := range fields {
		// Not .(mSA) because type assertion of an unnamed type to a named type always fails (the types
		// are not "identical"), but the assignment is fine because they are "assignable".
		m = m[field].(map[string]any)
	}
	return m
}

func TestNewUntrustedSigstorePayload(t *testing.T) {
	timeBefore := time.Now()
	sig := NewUntrustedSigstorePayload(TestImageManifestDigest, TestImageSignatureReference)
	assert.Equal(t, TestImageManifestDigest, sig.untrustedDockerManifestDigest)
	assert.Equal(t, TestImageSignatureReference, sig.untrustedDockerReference)
	require.NotNil(t, sig.untrustedCreatorID)
	assert.Equal(t, "containers/image "+version.Version, *sig.untrustedCreatorID)
	require.NotNil(t, sig.untrustedTimestamp)
	timeAfter := time.Now()
	assert.True(t, timeBefore.Unix() <= *sig.untrustedTimestamp)
	assert.True(t, *sig.untrustedTimestamp <= timeAfter.Unix())
}

func TestUntrustedSigstorePayloadMarshalJSON(t *testing.T) {
	const testDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

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
				untrustedDockerManifestDigest: testDigest,
				untrustedDockerReference:      "reference#@!",
				untrustedCreatorID:            &creatorID,
				untrustedTimestamp:            &timestamp,
			},
			"{\"critical\":{\"identity\":{\"docker-reference\":\"reference#@!\"},\"image\":{\"docker-manifest-digest\":\"" + testDigest + "\"},\"type\":\"cosign container image signature\"},\"optional\":{\"creator\":\"CREATOR\",\"timestamp\":1484683104}}",
		},
		{
			UntrustedSigstorePayload{
				untrustedDockerManifestDigest: testDigest,
				untrustedDockerReference:      "reference#@!",
			},
			"{\"critical\":{\"identity\":{\"docker-reference\":\"reference#@!\"},\"image\":{\"docker-manifest-digest\":\"" + testDigest + "\"},\"type\":\"cosign container image signature\"},\"optional\":{}}",
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
	const testDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	// Invalid input. Note that json.Unmarshal is guaranteed to validate input before calling our
	// UnmarshalJSON implementation; so test that first, then test our error handling for completeness.
	assertUnmarshalUntrustedSigstorePayloadFails(t, []byte("&"))
	var s UntrustedSigstorePayload
	err := s.UnmarshalJSON([]byte("&"))
	assert.Error(t, err)

	// Not an object
	assertUnmarshalUntrustedSigstorePayloadFails(t, []byte("1"))

	// Start with a valid JSON.
	validSig := NewUntrustedSigstorePayload(testDigest, "reference#@!")
	validJSON, err := validSig.MarshalJSON()
	require.NoError(t, err)

	// Success
	s = successfullyUnmarshalUntrustedSigstorePayload(t, validJSON)
	assert.Equal(t, validSig, s)

	// A /usr/bin/cosign-generated payload is handled correctly
	s = successfullyUnmarshalUntrustedSigstorePayload(t, []byte(`{"critical":{"identity":{"docker-reference":"192.168.64.2:5000/cosign-signed-multi"},"image":{"docker-manifest-digest":"sha256:43955d6857268cc948ae9b370b221091057de83c4962da0826f9a2bdc9bd6b44"},"type":"cosign container image signature"},"optional":null}`))
	assert.Equal(t, UntrustedSigstorePayload{
		untrustedDockerManifestDigest: "sha256:43955d6857268cc948ae9b370b221091057de83c4962da0826f9a2bdc9bd6b44",
		untrustedDockerReference:      "192.168.64.2:5000/cosign-signed-multi",
		untrustedCreatorID:            nil,
		untrustedTimestamp:            nil,
	}, s)

	// Various ways to corrupt the JSON
	breakFns := []func(mSA){
		// A top-level field is missing
		func(v mSA) { delete(v, "critical") },
		func(v mSA) { delete(v, "optional") },
		// Extra top-level sub-object
		func(v mSA) { v["unexpected"] = 1 },
		// "critical" not an object
		func(v mSA) { v["critical"] = 1 },
		// "optional" not an object
		func(v mSA) { v["optional"] = 1 },
		// A field of "critical" is missing
		func(v mSA) { delete(x(v, "critical"), "type") },
		func(v mSA) { delete(x(v, "critical"), "image") },
		func(v mSA) { delete(x(v, "critical"), "identity") },
		// Extra field of "critical"
		func(v mSA) { x(v, "critical")["unexpected"] = 1 },
		// Invalid "type"
		func(v mSA) { x(v, "critical")["type"] = 1 },
		func(v mSA) { x(v, "critical")["type"] = "unexpected" },
		// Invalid "image" object
		func(v mSA) { x(v, "critical")["image"] = 1 },
		func(v mSA) { delete(x(v, "critical", "image"), "docker-manifest-digest") },
		func(v mSA) { x(v, "critical", "image")["unexpected"] = 1 },
		// Invalid "docker-manifest-digest"
		func(v mSA) { x(v, "critical", "image")["docker-manifest-digest"] = 1 },
		func(v mSA) { x(v, "critical", "image")["docker-manifest-digest"] = "sha256:../.." },
		// Invalid "identity" object
		func(v mSA) { x(v, "critical")["identity"] = 1 },
		func(v mSA) { delete(x(v, "critical", "identity"), "docker-reference") },
		func(v mSA) { x(v, "critical", "identity")["unexpected"] = 1 },
		// Invalid "docker-reference"
		func(v mSA) { x(v, "critical", "identity")["docker-reference"] = 1 },
		// Invalid "creator"
		func(v mSA) { x(v, "optional")["creator"] = 1 },
		// Invalid "timestamp"
		func(v mSA) { x(v, "optional")["timestamp"] = "unexpected" },
		func(v mSA) { x(v, "optional")["timestamp"] = 0.5 }, // Fractional input
	}
	for _, fn := range breakFns {
		testJSON := modifiedJSON(t, validJSON, fn)
		assertUnmarshalUntrustedSigstorePayloadFails(t, testJSON)
	}

	// Modifications to unrecognized fields in "optional" are allowed and ignored
	allowedModificationFns := []func(mSA){
		// Add an optional field
		func(v mSA) { x(v, "optional")["unexpected"] = 1 },
	}
	for _, fn := range allowedModificationFns {
		testJSON := modifiedJSON(t, validJSON, fn)
		s := successfullyUnmarshalUntrustedSigstorePayload(t, testJSON)
		assert.Equal(t, validSig, s)
	}

	// Optional fields can be missing
	validSig = UntrustedSigstorePayload{
		untrustedDockerManifestDigest: testDigest,
		untrustedDockerReference:      "reference#@!",
		untrustedCreatorID:            nil,
		untrustedTimestamp:            nil,
	}
	validJSON, err = validSig.MarshalJSON()
	require.NoError(t, err)
	s = successfullyUnmarshalUntrustedSigstorePayload(t, validJSON)
	assert.Equal(t, validSig, s)
}

// verifySigstorePayloadBlobSignature is tested by TestVerifySigstorePayload

func TestVerifySigstorePayload(t *testing.T) {
	publicKeyPEM, err := os.ReadFile("./testdata/cosign.pub")
	require.NoError(t, err)
	publicKey, err := cryptoutils.UnmarshalPEMToPublicKey(publicKeyPEM)
	require.NoError(t, err)
	publicKeyPEM2, err := os.ReadFile("./testdata/cosign2.pub")
	require.NoError(t, err)
	publicKey2, err := cryptoutils.UnmarshalPEMToPublicKey(publicKeyPEM2)
	require.NoError(t, err)
	singlePublicKey := []crypto.PublicKey{publicKey}

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
	for _, publicKeys := range [][]crypto.PublicKey{
		singlePublicKey,
		{publicKey, publicKey2}, // The matching key is first
		{publicKey2, publicKey}, // The matching key is second
	} {
		wanted = signatureData
		recorded = acceptanceData{}
		res, err := VerifySigstorePayload(publicKeys, sigstoreSig.UntrustedPayload(), cryptoBase64Sig, recordingRules)
		require.NoError(t, err)
		assert.Equal(t, res, &UntrustedSigstorePayload{
			untrustedDockerManifestDigest: TestSigstoreManifestDigest,
			untrustedDockerReference:      TestSigstoreSignatureReference,
			untrustedCreatorID:            nil,
			untrustedTimestamp:            nil,
		})
		assert.Equal(t, signatureData, recorded)
	}

	// For extra paranoia, test that we return a nil signature object on error.

	// Invalid base64 encoding
	for _, invalidBase64Sig := range []string{
		"&",                                      // Invalid base64 characters
		cryptoBase64Sig + "=",                    // Extra padding
		cryptoBase64Sig[:len(cryptoBase64Sig)-1], // Truncated base64 data
	} {
		recorded = acceptanceData{}
		res, err := VerifySigstorePayload(singlePublicKey, sigstoreSig.UntrustedPayload(), invalidBase64Sig, recordingRules)
		assert.Error(t, err)
		assert.Nil(t, res)
		assert.Equal(t, acceptanceData{}, recorded)
	}

	// No public keys
	recorded = acceptanceData{}
	res, err := VerifySigstorePayload([]crypto.PublicKey{}, sigstoreSig.UntrustedPayload(), cryptoBase64Sig, recordingRules)
	assert.Error(t, err)
	assert.Nil(t, res)
	assert.Equal(t, acceptanceData{}, recorded)

	// Invalid verifier:
	// crypto.PublicKey is, for some reason, just an any, so using a struct{}{} to trigger this code path works.
	for _, invalidPublicKeys := range [][]crypto.PublicKey{
		{struct{}{}},            // A single invalid key
		{struct{}{}, publicKey}, // An invalid key, followed by a matching key
		{publicKey, struct{}{}}, // A matching key, but the configuration also includes an invalid key
	} {
		recorded = acceptanceData{}
		res, err = VerifySigstorePayload(invalidPublicKeys, sigstoreSig.UntrustedPayload(), cryptoBase64Sig, recordingRules)
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
		append(bytes.Clone(validSignatureBytes), validSignatureBytes...),
	} {
		recorded = acceptanceData{}
		res, err = VerifySigstorePayload(singlePublicKey, sigstoreSig.UntrustedPayload(), base64.StdEncoding.EncodeToString(invalidSig), recordingRules)
		assert.Error(t, err)
		assert.Nil(t, res)
		assert.Equal(t, acceptanceData{}, recorded)
	}

	// No matching public keys
	for _, nonmatchingPublicKeys := range [][]crypto.PublicKey{
		{publicKey2},
		{publicKey2, publicKey2},
	} {
		recorded = acceptanceData{}
		res, err = VerifySigstorePayload(nonmatchingPublicKeys, sigstoreSig.UntrustedPayload(), cryptoBase64Sig, recordingRules)
		assert.Error(t, err)
		assert.Nil(t, res)
		assert.Equal(t, acceptanceData{}, recorded)
	}

	// Valid signature of non-JSON
	recorded = acceptanceData{}
	res, err = VerifySigstorePayload(singlePublicKey, []byte("&"), "MEUCIARnnxZQPALBfqkB4aNAYXad79Qs6VehcrgIeZ8p7I2FAiEAzq2HXwXlz1iJeh+ucUR3L0zpjynQk6Rk0+/gXYp49RU=", recordingRules)
	assert.Error(t, err)
	assert.Nil(t, res)
	assert.Equal(t, acceptanceData{}, recorded)

	// Valid signature of an unacceptable JSON
	recorded = acceptanceData{}
	res, err = VerifySigstorePayload(singlePublicKey, []byte("{}"), "MEUCIQDkySOBGxastVP0+koTA33NH5hXjwosFau4rxTPN6g48QIgb7eWKkGqfEpHMM3aT4xiqyP/170jEkdFuciuwN4mux4=", recordingRules)
	assert.Error(t, err)
	assert.Nil(t, res)
	assert.Equal(t, acceptanceData{}, recorded)

	// Valid signature with a wrong manifest digest: asked for signedDockerManifestDigest
	wanted = signatureData
	wanted.signedDockerManifestDigest = "invalid digest"
	recorded = acceptanceData{}
	res, err = VerifySigstorePayload(singlePublicKey, sigstoreSig.UntrustedPayload(), cryptoBase64Sig, recordingRules)
	assert.Error(t, err)
	assert.Nil(t, res)
	assert.Equal(t, acceptanceData{
		signedDockerManifestDigest: signatureData.signedDockerManifestDigest,
	}, recorded)

	// Valid signature with a wrong image reference
	wanted = signatureData
	wanted.signedDockerReference = "unexpected docker reference"
	recorded = acceptanceData{}
	res, err = VerifySigstorePayload(singlePublicKey, sigstoreSig.UntrustedPayload(), cryptoBase64Sig, recordingRules)
	assert.Error(t, err)
	assert.Nil(t, res)
	assert.Equal(t, signatureData, recorded)
}

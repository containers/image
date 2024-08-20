//go:build !containers_image_rekor_stub
// +build !containers_image_rekor_stub

package internal

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/go-openapi/strfmt"
	"github.com/sigstore/rekor/pkg/generated/models"
	"github.com/sigstore/sigstore/pkg/cryptoutils"
	sigstoreSignature "github.com/sigstore/sigstore/pkg/signature"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Verify that input can be unmarshaled as an UntrustedRekorSET.
func successfullyUnmarshalUntrustedRekorSET(t *testing.T, input []byte) UntrustedRekorSET {
	var s UntrustedRekorSET
	err := json.Unmarshal(input, &s)
	require.NoError(t, err, string(input))

	return s
}

// Verify that input can't be unmarshaled as an UntrustedRekorSET.
func assertUnmarshalUntrustedRekorSETFails(t *testing.T, input []byte) {
	var s UntrustedRekorSET
	err := json.Unmarshal(input, &s)
	assert.Error(t, err, string(input))
}

func TestUntrustedRekorSETUnmarshalJSON(t *testing.T) {
	// Invalid input. Note that json.Unmarshal is guaranteed to validate input before calling our
	// UnmarshalJSON implementation; so test that first, then test our error handling for completeness.
	assertUnmarshalUntrustedRekorSETFails(t, []byte("&"))
	var s UntrustedRekorSET
	err := s.UnmarshalJSON([]byte("&"))
	assert.Error(t, err)

	// Not an object
	assertUnmarshalUntrustedRekorSETFails(t, []byte("1"))

	// Start with a valid JSON.
	validSET := UntrustedRekorSET{
		UntrustedSignedEntryTimestamp: []byte("signedTimestamp#@!"),
		UntrustedPayload:              json.RawMessage(`["payload#@!"]`),
	}
	validJSON, err := json.Marshal(validSET)
	require.NoError(t, err)

	// Success
	s = successfullyUnmarshalUntrustedRekorSET(t, validJSON)
	assert.Equal(t, validSET, s)

	// A /usr/bin/cosign-generated payload is handled correctly
	setBytes, err := os.ReadFile("testdata/rekor-set")
	require.NoError(t, err)
	s = successfullyUnmarshalUntrustedRekorSET(t, setBytes)
	expectedSET, err := base64.StdEncoding.DecodeString(`MEYCIQDdeujdGLpMTgFdew9wsSJ3WF7olX9PawgzGeX2RmJd8QIhAPxGJf+HjUFVpQc0hgPaUSK8LsONJ08fZFEBVKDeLj4S`)
	require.NoError(t, err)
	assert.Equal(t, UntrustedRekorSET{
		UntrustedSignedEntryTimestamp: expectedSET,
		UntrustedPayload:              []byte(`{"body":"eyJhcGlWZXJzaW9uIjoiMC4wLjEiLCJraW5kIjoiaGFzaGVkcmVrb3JkIiwic3BlYyI6eyJkYXRhIjp7Imhhc2giOnsiYWxnb3JpdGhtIjoic2hhMjU2IiwidmFsdWUiOiIwZmQxZTk4MzJjYzVhNWY1MDJlODAwZmU5Y2RlZWZiZDMxMzYyZGYxNmZlOGMyMjUwZDMwOGFlYTNmYjFmYzY5In19LCJzaWduYXR1cmUiOnsiY29udGVudCI6Ik1FUUNJRUpjOTZlMDQxVkFoS0EwM1N2ZkNZYldvZElNSVFQeUF0V3lEUDRGblBxcEFpQWFJUzYwRWpoUkRoU2Fub0Zzb0l5OGZLcXFLZVc1cHMvdExYU0dwYXlpMmc9PSIsInB1YmxpY0tleSI6eyJjb250ZW50IjoiTFMwdExTMUNSVWRKVGlCRFJWSlVTVVpKUTBGVVJTMHRMUzB0Q2sxSlNVTnVWRU5EUVdsUFowRjNTVUpCWjBsVlJ6UTFkV0ZETW5vNFZuWjFUM2Q2ZW0wM09WSlFabU5yYjNoM2QwTm5XVWxMYjFwSmVtb3dSVUYzVFhjS1RucEZWazFDVFVkQk1WVkZRMmhOVFdNeWJHNWpNMUoyWTIxVmRWcEhWakpOVWpSM1NFRlpSRlpSVVVSRmVGWjZZVmRrZW1SSE9YbGFVekZ3WW01U2JBcGpiVEZzV2tkc2FHUkhWWGRJYUdOT1RXcEplRTFxUlhsTlZHY3dUMFJGTkZkb1kwNU5ha2w0VFdwRmVVMVVaekZQUkVVMFYycEJRVTFHYTNkRmQxbElDa3R2V2tsNmFqQkRRVkZaU1V0dldrbDZhakJFUVZGalJGRm5RVVZLUW5WeWEyaFRVbTFJZFd0SVZtZ3pWa0U0YmxsMVUxZHhXalJzZEdGbVJIZDVWMndLWXpOak9VNWhURzkyYlhVclRrTTBUbWxWZEUxTWFXWk1MMUF6Ym5GbFpHSnVZM1JMUW5WWmJXWkpVMGRwV214V2VVdFBRMEZWU1hkblowVXJUVUUwUndwQk1WVmtSSGRGUWk5M1VVVkJkMGxJWjBSQlZFSm5UbFpJVTFWRlJFUkJTMEpuWjNKQ1owVkdRbEZqUkVGNlFXUkNaMDVXU0ZFMFJVWm5VVlZCYlhaSENqUkxObXhPYXk5emF5OW1OR0ZwWVdocVdWSnhVaXRqZDBoM1dVUldVakJxUWtKbmQwWnZRVlV6T1ZCd2VqRlphMFZhWWpWeFRtcHdTMFpYYVhocE5Ga0tXa1E0ZDBoUldVUldVakJTUVZGSUwwSkNUWGRGV1VWUVlsZHNNR05yUW5sYVYxSnZXVmhSZFZreU9YUk5RM2RIUTJselIwRlJVVUpuTnpoM1FWRkZSUXBJYldnd1pFaENlazlwT0haYU1td3dZVWhXYVV4dFRuWmlVemx6WWpKa2NHSnBPWFpaV0ZZd1lVUkRRbWxSV1V0TGQxbENRa0ZJVjJWUlNVVkJaMUkzQ2tKSWEwRmtkMEl4UVU0d09VMUhja2Q0ZUVWNVdYaHJaVWhLYkc1T2QwdHBVMncyTkROcWVYUXZOR1ZMWTI5QmRrdGxOazlCUVVGQ2FGRmxjV3gzVlVFS1FVRlJSRUZGV1hkU1FVbG5WbWt5VTNaT05WSmxSMWxwVDFOb1dUaE1SbE5TUnpWRU5tOUJWRXR4U0dJMmEwNHZSRXBvTW5KQlZVTkpRVTVUTmtGeGNBcDRZVmhwU0hkVVNGVnlNM2hRVTBkaE5XazJhSGwzYldKaVVrTTJUakJyU1dWRVRUWk5RVzlIUTBOeFIxTk5ORGxDUVUxRVFUSm5RVTFIVlVOTlEweDFDbU5hZEVWVFNVNHdiRzAyTkVOdkwySmFOamhEUTFKclYyeHJkRmcwYlcxS2FWSm9TMms1WXpsUlJEWXlRelZUZFZwb1l6QjJkbTgyVFU5TGJWUlJTWGdLUVVsdkwwMXZlbHBsYjFVM2NtUk9hakJ3V2t0MVFtVkRiVTF4YlVwaFJGTnpkekU1ZEV0cEwySXhjRVZ0ZFhjclUyWXlRa2t5TlVkblNXSkxlblJITVFvNWR6MDlDaTB0TFMwdFJVNUVJRU5GVWxSSlJrbERRVlJGTFMwdExTMEsifX19fQ==","integratedTime":1670870899,"logIndex":8949589,"logID":"c0d23d6ad406973f9559f3ba2d1ca01f84147d8ffc5b8445c224f98b9591801d"}`),
	}, s)

	// Various ways to corrupt the JSON
	breakFns := []func(mSA){
		// A top-level field is missing
		func(v mSA) { delete(v, "SignedEntryTimestamp") },
		func(v mSA) { delete(v, "Payload") },
		// Extra top-level sub-object
		func(v mSA) { v["unexpected"] = 1 },
		// "SignedEntryTimestamp" not a string
		func(v mSA) { v["critical"] = 1 },
		// "Payload" not an object
		func(v mSA) { v["optional"] = 1 },
	}
	for _, fn := range breakFns {
		testJSON := modifiedJSON(t, validJSON, fn)
		assertUnmarshalUntrustedRekorSETFails(t, testJSON)
	}
}

// Verify that input can be unmarshaled as an UntrustedRekorPayload.
func successfullyUnmarshalUntrustedRekorPayload(t *testing.T, input []byte) UntrustedRekorPayload {
	var s UntrustedRekorPayload
	err := json.Unmarshal(input, &s)
	require.NoError(t, err, string(input))

	return s
}

// Verify that input can't be unmarshaled as an UntrustedRekorPayload.
func assertUnmarshalUntrustedRekorPayloadFails(t *testing.T, input []byte) {
	var s UntrustedRekorPayload
	err := json.Unmarshal(input, &s)
	assert.Error(t, err, string(input))
}

func TestUntrustedRekorPayloadUnmarshalJSON(t *testing.T) {
	// Invalid input. Note that json.Unmarshal is guaranteed to validate input before calling our
	// UnmarshalJSON implementation; so test that first, then test our error handling for completeness.
	assertUnmarshalUntrustedRekorPayloadFails(t, []byte("&"))
	var p UntrustedRekorPayload
	err := p.UnmarshalJSON([]byte("&"))
	assert.Error(t, err)

	// Not an object
	assertUnmarshalUntrustedRekorPayloadFails(t, []byte("1"))

	// Start with a valid JSON.
	validPayload := UntrustedRekorPayload{
		Body:           []byte(`["json"]`),
		IntegratedTime: 1,
		LogIndex:       2,
		LogID:          "abc",
	}
	validJSON, err := validPayload.MarshalJSON()
	require.NoError(t, err)

	// Success
	p = successfullyUnmarshalUntrustedRekorPayload(t, validJSON)
	assert.Equal(t, validPayload, p)

	// A /usr/bin/cosign-generated payload is handled correctly
	setBytes, err := os.ReadFile("testdata/rekor-set")
	require.NoError(t, err)
	s := successfullyUnmarshalUntrustedRekorSET(t, setBytes)
	p = successfullyUnmarshalUntrustedRekorPayload(t, s.UntrustedPayload)
	expectedBody, err := base64.StdEncoding.DecodeString(`eyJhcGlWZXJzaW9uIjoiMC4wLjEiLCJraW5kIjoiaGFzaGVkcmVrb3JkIiwic3BlYyI6eyJkYXRhIjp7Imhhc2giOnsiYWxnb3JpdGhtIjoic2hhMjU2IiwidmFsdWUiOiIwZmQxZTk4MzJjYzVhNWY1MDJlODAwZmU5Y2RlZWZiZDMxMzYyZGYxNmZlOGMyMjUwZDMwOGFlYTNmYjFmYzY5In19LCJzaWduYXR1cmUiOnsiY29udGVudCI6Ik1FUUNJRUpjOTZlMDQxVkFoS0EwM1N2ZkNZYldvZElNSVFQeUF0V3lEUDRGblBxcEFpQWFJUzYwRWpoUkRoU2Fub0Zzb0l5OGZLcXFLZVc1cHMvdExYU0dwYXlpMmc9PSIsInB1YmxpY0tleSI6eyJjb250ZW50IjoiTFMwdExTMUNSVWRKVGlCRFJWSlVTVVpKUTBGVVJTMHRMUzB0Q2sxSlNVTnVWRU5EUVdsUFowRjNTVUpCWjBsVlJ6UTFkV0ZETW5vNFZuWjFUM2Q2ZW0wM09WSlFabU5yYjNoM2QwTm5XVWxMYjFwSmVtb3dSVUYzVFhjS1RucEZWazFDVFVkQk1WVkZRMmhOVFdNeWJHNWpNMUoyWTIxVmRWcEhWakpOVWpSM1NFRlpSRlpSVVVSRmVGWjZZVmRrZW1SSE9YbGFVekZ3WW01U2JBcGpiVEZzV2tkc2FHUkhWWGRJYUdOT1RXcEplRTFxUlhsTlZHY3dUMFJGTkZkb1kwNU5ha2w0VFdwRmVVMVVaekZQUkVVMFYycEJRVTFHYTNkRmQxbElDa3R2V2tsNmFqQkRRVkZaU1V0dldrbDZhakJFUVZGalJGRm5RVVZLUW5WeWEyaFRVbTFJZFd0SVZtZ3pWa0U0YmxsMVUxZHhXalJzZEdGbVJIZDVWMndLWXpOak9VNWhURzkyYlhVclRrTTBUbWxWZEUxTWFXWk1MMUF6Ym5GbFpHSnVZM1JMUW5WWmJXWkpVMGRwV214V2VVdFBRMEZWU1hkblowVXJUVUUwUndwQk1WVmtSSGRGUWk5M1VVVkJkMGxJWjBSQlZFSm5UbFpJVTFWRlJFUkJTMEpuWjNKQ1owVkdRbEZqUkVGNlFXUkNaMDVXU0ZFMFJVWm5VVlZCYlhaSENqUkxObXhPYXk5emF5OW1OR0ZwWVdocVdWSnhVaXRqZDBoM1dVUldVakJxUWtKbmQwWnZRVlV6T1ZCd2VqRlphMFZhWWpWeFRtcHdTMFpYYVhocE5Ga0tXa1E0ZDBoUldVUldVakJTUVZGSUwwSkNUWGRGV1VWUVlsZHNNR05yUW5sYVYxSnZXVmhSZFZreU9YUk5RM2RIUTJselIwRlJVVUpuTnpoM1FWRkZSUXBJYldnd1pFaENlazlwT0haYU1td3dZVWhXYVV4dFRuWmlVemx6WWpKa2NHSnBPWFpaV0ZZd1lVUkRRbWxSV1V0TGQxbENRa0ZJVjJWUlNVVkJaMUkzQ2tKSWEwRmtkMEl4UVU0d09VMUhja2Q0ZUVWNVdYaHJaVWhLYkc1T2QwdHBVMncyTkROcWVYUXZOR1ZMWTI5QmRrdGxOazlCUVVGQ2FGRmxjV3gzVlVFS1FVRlJSRUZGV1hkU1FVbG5WbWt5VTNaT05WSmxSMWxwVDFOb1dUaE1SbE5TUnpWRU5tOUJWRXR4U0dJMmEwNHZSRXBvTW5KQlZVTkpRVTVUTmtGeGNBcDRZVmhwU0hkVVNGVnlNM2hRVTBkaE5XazJhSGwzYldKaVVrTTJUakJyU1dWRVRUWk5RVzlIUTBOeFIxTk5ORGxDUVUxRVFUSm5RVTFIVlVOTlEweDFDbU5hZEVWVFNVNHdiRzAyTkVOdkwySmFOamhEUTFKclYyeHJkRmcwYlcxS2FWSm9TMms1WXpsUlJEWXlRelZUZFZwb1l6QjJkbTgyVFU5TGJWUlJTWGdLUVVsdkwwMXZlbHBsYjFVM2NtUk9hakJ3V2t0MVFtVkRiVTF4YlVwaFJGTnpkekU1ZEV0cEwySXhjRVZ0ZFhjclUyWXlRa2t5TlVkblNXSkxlblJITVFvNWR6MDlDaTB0TFMwdFJVNUVJRU5GVWxSSlJrbERRVlJGTFMwdExTMEsifX19fQ==`)
	require.NoError(t, err)
	assert.Equal(t, UntrustedRekorPayload{
		Body:           expectedBody,
		IntegratedTime: 1670870899,
		LogIndex:       8949589,
		LogID:          "c0d23d6ad406973f9559f3ba2d1ca01f84147d8ffc5b8445c224f98b9591801d",
	}, p)

	// Various ways to corrupt the JSON
	breakFns := []func(mSA){
		// A top-level field is missing
		func(v mSA) { delete(v, "body") },
		func(v mSA) { delete(v, "integratedTime") },
		func(v mSA) { delete(v, "logIndex") },
		func(v mSA) { delete(v, "logID") },
		// Extra top-level sub-object
		func(v mSA) { v["unexpected"] = 1 },
		// "body" not a string
		func(v mSA) { v["body"] = 1 },
		// "integratedTime" not an integer
		func(v mSA) { v["integratedTime"] = "hello" },
		// "logIndex" not an integer
		func(v mSA) { v["logIndex"] = "hello" },
		// "logID" not a string
		func(v mSA) { v["logID"] = 1 },
	}
	for _, fn := range breakFns {
		testJSON := modifiedJSON(t, validJSON, fn)
		assertUnmarshalUntrustedRekorPayloadFails(t, testJSON)
	}
}

// stringPtr returns a pointer to the provided string value.
func stringPtr(s string) *string {
	return &s
}

func TestVerifyRekorSET(t *testing.T) {
	cosignRekorKeyPEM, err := os.ReadFile("testdata/rekor.pub")
	require.NoError(t, err)
	cosignRekorKey, err := cryptoutils.UnmarshalPEMToPublicKey(cosignRekorKeyPEM)
	require.NoError(t, err)
	cosignRekorKeyECDSA, ok := cosignRekorKey.(*ecdsa.PublicKey)
	require.True(t, ok)
	cosignRekorKeysECDSA := []*ecdsa.PublicKey{cosignRekorKeyECDSA}
	cosignSETBytes, err := os.ReadFile("testdata/rekor-set")
	require.NoError(t, err)
	cosignCertBytes, err := os.ReadFile("testdata/rekor-cert")
	require.NoError(t, err)
	cosignSigBase64, err := os.ReadFile("testdata/rekor-sig")
	require.NoError(t, err)
	cosignPayloadBytes, err := os.ReadFile("testdata/rekor-payload")
	require.NoError(t, err)
	mismatchingKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader) // A key which did not sign anything
	require.NoError(t, err)

	// Successful verification
	for _, acceptableKeys := range [][]*ecdsa.PublicKey{
		{cosignRekorKeyECDSA},
		{cosignRekorKeyECDSA, &mismatchingKey.PublicKey},
		{&mismatchingKey.PublicKey, cosignRekorKeyECDSA},
	} {
		tm, err := VerifyRekorSET(acceptableKeys, cosignSETBytes, cosignCertBytes, string(cosignSigBase64), cosignPayloadBytes)
		require.NoError(t, err)
		assert.Equal(t, time.Unix(1670870899, 0), tm)
	}

	// For extra paranoia, test that we return a zero time on error.

	// A completely invalid SET.
	tm, err := VerifyRekorSET(cosignRekorKeysECDSA, []byte{}, cosignCertBytes, string(cosignSigBase64), cosignPayloadBytes)
	assert.Error(t, err)
	assert.Zero(t, tm)

	tm, err = VerifyRekorSET(cosignRekorKeysECDSA, []byte("invalid signature"), cosignCertBytes, string(cosignSigBase64), cosignPayloadBytes)
	assert.Error(t, err)
	assert.Zero(t, tm)

	testKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	testPublicKeys := []*ecdsa.PublicKey{&testKey.PublicKey}
	testSigner, err := sigstoreSignature.LoadECDSASigner(testKey, crypto.SHA256)
	require.NoError(t, err)

	// JSON canonicalization fails:
	// This payload is invalid because it has duplicate fields.
	// Right now, that particular failure (unlike more blatantly invalid JSON) is allowed
	// by json.Marshal, but detected by jsoncanonicalizer.Transform.
	invalidPayload := []byte(`{"logIndex":1, "integratedTime":2,"body":"abc","logID":"def","body":"ABC"}`)
	invalidPayloadSig, err := testSigner.SignMessage(bytes.NewReader(invalidPayload))
	require.NoError(t, err)
	invalidSET, err := json.Marshal(UntrustedRekorSET{
		UntrustedSignedEntryTimestamp: invalidPayloadSig,
		UntrustedPayload:              json.RawMessage(invalidPayload),
	})
	require.NoError(t, err)
	tm, err = VerifyRekorSET(testPublicKeys, invalidSET, cosignCertBytes, string(cosignSigBase64), cosignPayloadBytes)
	assert.Error(t, err)
	assert.Zero(t, tm)

	// Cryptographic verification fails (a mismatched public key)
	for _, mismatchingKeys := range [][]*ecdsa.PublicKey{
		{&testKey.PublicKey},
		{&testKey.PublicKey, &mismatchingKey.PublicKey},
	} {
		tm, err := VerifyRekorSET(mismatchingKeys, cosignSETBytes, cosignCertBytes, string(cosignSigBase64), cosignPayloadBytes)
		assert.Error(t, err)
		assert.Zero(t, tm)
	}

	// Parsing UntrustedRekorPayload fails
	invalidPayload = []byte(`{}`)
	invalidPayloadSig, err = testSigner.SignMessage(bytes.NewReader(invalidPayload))
	require.NoError(t, err)
	invalidSET, err = json.Marshal(UntrustedRekorSET{
		UntrustedSignedEntryTimestamp: invalidPayloadSig,
		UntrustedPayload:              json.RawMessage(invalidPayload),
	})
	require.NoError(t, err)
	tm, err = VerifyRekorSET(testPublicKeys, invalidSET, cosignCertBytes, string(cosignSigBase64), cosignPayloadBytes)
	assert.Error(t, err)
	assert.Zero(t, tm)

	// A correctly signed UntrustedRekorPayload is invalid
	cosignPayloadSHA256 := sha256.Sum256(cosignPayloadBytes)
	cosignSigBytes, err := base64.StdEncoding.DecodeString(string(cosignSigBase64))
	require.NoError(t, err)
	validHashedRekord := models.Hashedrekord{
		APIVersion: stringPtr(HashedRekordV001APIVersion),
		Spec: models.HashedrekordV001Schema{
			Data: &models.HashedrekordV001SchemaData{
				Hash: &models.HashedrekordV001SchemaDataHash{
					Algorithm: stringPtr(models.HashedrekordV001SchemaDataHashAlgorithmSha256),
					Value:     stringPtr(hex.EncodeToString(cosignPayloadSHA256[:])),
				},
			},
			Signature: &models.HashedrekordV001SchemaSignature{
				Content: strfmt.Base64(cosignSigBytes),
				PublicKey: &models.HashedrekordV001SchemaSignaturePublicKey{
					Content: strfmt.Base64(cosignCertBytes),
				},
			},
		},
	}
	validHashedRekordJSON, err := json.Marshal(validHashedRekord)
	require.NoError(t, err)
	for _, fn := range []func(mSA){
		// A Hashedrekord field is missing
		func(v mSA) { delete(v, "apiVersion") },
		func(v mSA) { delete(v, "kind") }, // "kind" is not visible in the type definition, but required by the implementation
		func(v mSA) { delete(v, "spec") },
		// This, along with many other extra fields, is currently accepted. That is NOT an API commitment.
		// func(v mSA) { v["unexpected"] = 1 }, // Extra top-level field:
		// Invalid apiVersion
		func(v mSA) { v["apiVersion"] = nil },
		func(v mSA) { v["apiVersion"] = 1 },
		func(v mSA) { v["apiVersion"] = mSA{} },
		func(v mSA) { v["apiVersion"] = "99.0.99" },
		// Invalid kind
		func(v mSA) { v["kind"] = nil },
		func(v mSA) { v["kind"] = 1 },
		func(v mSA) { v["kind"] = "notHashedRekord" },
		// Invalid spec
		func(v mSA) { v["spec"] = nil },
		func(v mSA) { v["spec"] = 1 },
		// A HashedRekordV001Schema field is missing
		func(v mSA) { delete(x(v, "spec"), "data") },
		func(v mSA) { delete(x(v, "spec"), "signature") },
		// Invalid spec.data
		func(v mSA) { x(v, "spec")["data"] = nil },
		func(v mSA) { x(v, "spec")["data"] = 1 },
		// Missing spec.data.hash
		func(v mSA) { delete(x(v, "spec", "data"), "hash") },
		// Invalid spec.data.hash
		func(v mSA) { x(v, "spec", "data")["hash"] = nil },
		func(v mSA) { x(v, "spec", "data")["hash"] = 1 },
		// A spec.data.hash field is missing
		func(v mSA) { delete(x(v, "spec", "data", "hash"), "algorithm") },
		func(v mSA) { delete(x(v, "spec", "data", "hash"), "value") },
		// Invalid spec.data.hash.algorithm
		func(v mSA) { x(v, "spec", "data", "hash")["algorithm"] = nil },
		func(v mSA) { x(v, "spec", "data", "hash")["algorithm"] = 1 },
		// Invalid spec.data.hash.value
		func(v mSA) { x(v, "spec", "data", "hash")["value"] = nil },
		func(v mSA) { x(v, "spec", "data", "hash")["value"] = 1 },
		func(v mSA) { // An odd number of hexadecimal digits
			x(v, "spec", "data", "hash")["value"] = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		},
		// spec.data.hash does not match
		func(v mSA) {
			x(v, "spec", "data", "hash")["value"] = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		},
		// A non-sha256 hash
		func(v mSA) {
			x(v, "spec", "data", "hash")["algorithm"] = "sha512"
			h := sha512.Sum512(cosignPayloadBytes)
			x(v, "spec", "data", "hash")["value"] = hex.EncodeToString(h[:])
		},
		// Invalid spec.signature
		func(v mSA) { x(v, "spec")["signature"] = nil },
		func(v mSA) { x(v, "spec")["signature"] = 1 },
		// A spec.signature field is missing
		func(v mSA) { delete(x(v, "spec", "signature"), "content") },
		func(v mSA) { delete(x(v, "spec", "signature"), "publicKey") },
		// Invalid spec.signature.content
		func(v mSA) { x(v, "spec", "signature")["content"] = nil },
		func(v mSA) { x(v, "spec", "signature")["content"] = 1 },
		func(v mSA) { x(v, "spec", "signature")["content"] = "" },
		func(v mSA) { x(v, "spec", "signature")["content"] = "+" }, // Invalid base64
		// spec.signature.content does not match
		func(v mSA) {
			x(v, "spec", "signature")["content"] = base64.StdEncoding.EncodeToString([]byte("does not match"))
		},
		// Invalid spec.signature.publicKey
		func(v mSA) { x(v, "spec", "signature")["publicKey"] = nil },
		func(v mSA) { x(v, "spec", "signature")["publicKey"] = 1 },
		// Missing spec.signature.publicKey.content
		func(v mSA) { delete(x(v, "spec", "signature", "publicKey"), "content") },
		// Invalid spec.signature.publicKey.content
		func(v mSA) { x(v, "spec", "signature", "publicKey")["content"] = nil },
		func(v mSA) { x(v, "spec", "signature", "publicKey")["content"] = 1 },
		func(v mSA) { x(v, "spec", "signature", "publicKey")["content"] = "" },
		func(v mSA) { x(v, "spec", "signature", "publicKey")["content"] = "+" }, // Invalid base64
		func(v mSA) {
			x(v, "spec", "signature", "publicKey")["content"] = base64.StdEncoding.EncodeToString([]byte("not PEM"))
		},
		func(v mSA) { // Multiple PEM blocks
			x(v, "spec", "signature", "publicKey")["content"] = base64.StdEncoding.EncodeToString(bytes.Repeat(cosignCertBytes, 2))
		},
		// spec.signature.publicKey.content does not match
		func(v mSA) {
			otherKey, err := testSigner.PublicKey()
			require.NoError(t, err)
			otherPEM, err := cryptoutils.MarshalPublicKeyToPEM(otherKey)
			require.NoError(t, err)
			x(v, "spec", "signature", "publicKey")["content"] = base64.StdEncoding.EncodeToString(otherPEM)
		},
	} {
		testHashedRekordJSON := modifiedJSON(t, validHashedRekordJSON, fn)
		testPayload, err := json.Marshal(UntrustedRekorPayload{
			Body:           testHashedRekordJSON,
			IntegratedTime: 1,
			LogIndex:       2,
			LogID:          "logID",
		})
		require.NoError(t, err)
		testPayloadSig, err := testSigner.SignMessage(bytes.NewReader(testPayload))
		require.NoError(t, err)
		testSET, err := json.Marshal(UntrustedRekorSET{
			UntrustedSignedEntryTimestamp: testPayloadSig,
			UntrustedPayload:              json.RawMessage(testPayload),
		})
		require.NoError(t, err)
		tm, err = VerifyRekorSET(testPublicKeys, testSET, cosignCertBytes, string(cosignSigBase64), cosignPayloadBytes)
		assert.Error(t, err)
		assert.Zero(t, tm)
	}

	// Invalid unverifiedBase64Signature parameter
	truncatedBase64 := cosignSigBase64
	truncatedBase64 = truncatedBase64[:len(truncatedBase64)-1]
	tm, err = VerifyRekorSET(cosignRekorKeysECDSA, cosignSETBytes, cosignCertBytes,
		string(truncatedBase64), cosignPayloadBytes)
	assert.Error(t, err)
	assert.Zero(t, tm)

	// Invalid unverifiedKeyOrCertBytes
	for _, c := range [][]byte{
		nil,
		{},
		[]byte("this is not PEM"),
		bytes.Repeat(cosignCertBytes, 2),
	} {
		tm, err = VerifyRekorSET(cosignRekorKeysECDSA, cosignSETBytes, c,
			string(cosignSigBase64), cosignPayloadBytes)
		assert.Error(t, err)
		assert.Zero(t, tm)
	}
}

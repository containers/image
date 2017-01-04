package gpgme

import (
	"io/ioutil"
	"testing"

	"github.com/containers/image/signature"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignDockerManifest(t *testing.T) {
	mech, err := newGPGSigningMechanismInDirectory(testGPGHomeDirectory)
	require.NoError(t, err)
	manifest, err := ioutil.ReadFile("../fixtures/image.manifest.json")
	require.NoError(t, err)

	// Successful signing
	s, err := signature.SignDockerManifest(manifest, signature.TestImageSignatureReference, mech, signature.TestKeyFingerprint)
	require.NoError(t, err)

	verified, err := signature.VerifyDockerManifestSignature(s, manifest, signature.TestImageSignatureReference, mech, signature.TestKeyFingerprint)
	assert.NoError(t, err)
	assert.Equal(t, signature.TestImageSignatureReference, verified.DockerReference)
	assert.Equal(t, signature.TestImageManifestDigest, verified.DockerManifestDigest)

	// Error computing Docker manifest
	invalidManifest, err := ioutil.ReadFile("../fixtures/v2s1-invalid-signatures.manifest.json")
	require.NoError(t, err)
	_, err = signature.SignDockerManifest(invalidManifest, signature.TestImageSignatureReference, mech, signature.TestKeyFingerprint)
	assert.Error(t, err)

	// Error creating blob to sign
	_, err = signature.SignDockerManifest(manifest, "", mech, signature.TestKeyFingerprint)
	assert.Error(t, err)

	// Error signing
	_, err = signature.SignDockerManifest(manifest, signature.TestImageSignatureReference, mech, "this fingerprint doesn't exist")
	assert.Error(t, err)
}

func TestVerifyDockerManifestSignature(t *testing.T) {
	mech, err := newGPGSigningMechanismInDirectory(testGPGHomeDirectory)
	require.NoError(t, err)
	manifest, err := ioutil.ReadFile("../fixtures/image.manifest.json")
	require.NoError(t, err)
	s, err := ioutil.ReadFile("../fixtures/image.signature")
	require.NoError(t, err)

	// Successful verification
	sig, err := signature.VerifyDockerManifestSignature(s, manifest, signature.TestImageSignatureReference, mech, signature.TestKeyFingerprint)
	require.NoError(t, err)
	assert.Equal(t, signature.TestImageSignatureReference, sig.DockerReference)
	assert.Equal(t, signature.TestImageManifestDigest, sig.DockerManifestDigest)

	// For extra paranoia, test that we return nil data on error.

	// Error computing Docker manifest
	invalidManifest, err := ioutil.ReadFile("../fixtures/v2s1-invalid-signatures.manifest.json")
	require.NoError(t, err)
	sig, err = signature.VerifyDockerManifestSignature(s, invalidManifest, signature.TestImageSignatureReference, mech, signature.TestKeyFingerprint)
	assert.Error(t, err)
	assert.Nil(t, sig)

	// Error verifying signature
	corruptSignature, err := ioutil.ReadFile("../fixtures/corrupt.signature")
	sig, err = signature.VerifyDockerManifestSignature(corruptSignature, manifest, signature.TestImageSignatureReference, mech, signature.TestKeyFingerprint)
	assert.Error(t, err)
	assert.Nil(t, sig)

	// Key fingerprint mismatch
	sig, err = signature.VerifyDockerManifestSignature(s, manifest, signature.TestImageSignatureReference, mech, "unexpected fingerprint")
	assert.Error(t, err)
	assert.Nil(t, sig)

	// Docker reference mismatch
	sig, err = signature.VerifyDockerManifestSignature(s, manifest, "example.com/doesnt/match", mech, signature.TestKeyFingerprint)
	assert.Error(t, err)
	assert.Nil(t, sig)

	// Docker manifest digest mismatch
	sig, err = signature.VerifyDockerManifestSignature(s, []byte("unexpected manifest"), signature.TestImageSignatureReference, mech, signature.TestKeyFingerprint)
	assert.Error(t, err)
	assert.Nil(t, sig)
}

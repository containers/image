package signature

import (
	"io/ioutil"
	"testing"

	"github.com/projectatomic/skopeo/signature/fixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDockerManifestDigest(t *testing.T) {
	manifest, err := ioutil.ReadFile("fixtures/image.manifest.json")
	require.NoError(t, err)
	digest := dockerManifestDigest(manifest)
	assert.Equal(t, fixtures.TestImageManifestDigest, digest)

	digest = dockerManifestDigest([]byte{})
	assert.Equal(t, "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", digest)
}

func TestSignDockerManifest(t *testing.T) {
	mech, err := newGPGSigningMechanismInDirectory(testGPGHomeDirectory)
	require.NoError(t, err)
	manifest, err := ioutil.ReadFile("fixtures/image.manifest.json")
	require.NoError(t, err)

	// Successful signing
	signature, err := SignDockerManifest(manifest, fixtures.TestImageSignatureReference, mech, fixtures.TestKeyFingerprint)
	require.NoError(t, err)

	verified, err := VerifyDockerManifestSignature(signature, manifest, fixtures.TestImageSignatureReference, mech, fixtures.TestKeyFingerprint)
	assert.NoError(t, err)
	assert.Equal(t, fixtures.TestImageSignatureReference, verified.DockerReference)
	assert.Equal(t, fixtures.TestImageManifestDigest, verified.DockerManifestDigest)

	// Error creating blob to sign
	_, err = SignDockerManifest(manifest, "", mech, fixtures.TestKeyFingerprint)
	assert.Error(t, err)

	// Error signing
	_, err = SignDockerManifest(manifest, fixtures.TestImageSignatureReference, mech, "this fingerprint doesn't exist")
	assert.Error(t, err)
}

func TestVerifyDockerManifestSignature(t *testing.T) {
	mech, err := newGPGSigningMechanismInDirectory(testGPGHomeDirectory)
	require.NoError(t, err)
	manifest, err := ioutil.ReadFile("fixtures/image.manifest.json")
	require.NoError(t, err)
	signature, err := ioutil.ReadFile("fixtures/image.signature")
	require.NoError(t, err)

	// Successful verification
	sig, err := VerifyDockerManifestSignature(signature, manifest, fixtures.TestImageSignatureReference, mech, fixtures.TestKeyFingerprint)
	require.NoError(t, err)
	assert.Equal(t, fixtures.TestImageSignatureReference, sig.DockerReference)
	assert.Equal(t, fixtures.TestImageManifestDigest, sig.DockerManifestDigest)

	// For extra paranoia, test that we return nil data on error.

	// Error verifying signature
	corruptSignature, err := ioutil.ReadFile("fixtures/corrupt.signature")
	sig, err = VerifyDockerManifestSignature(corruptSignature, manifest, fixtures.TestImageSignatureReference, mech, fixtures.TestKeyFingerprint)
	assert.Error(t, err)
	assert.Nil(t, sig)

	// Key fingerprint mismatch
	sig, err = VerifyDockerManifestSignature(signature, manifest, fixtures.TestImageSignatureReference, mech, "unexpected fingerprint")
	assert.Error(t, err)
	assert.Nil(t, sig)

	// Docker manifest digest mismatch
	sig, err = VerifyDockerManifestSignature(signature, []byte("unexpected manifest"), fixtures.TestImageSignatureReference, mech, fixtures.TestKeyFingerprint)
	assert.Error(t, err)
	assert.Nil(t, sig)
}

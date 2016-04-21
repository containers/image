package signature

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/projectatomic/skopeo/signature/fixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGuessManifestMIMEType(t *testing.T) {
	cases := []struct {
		path     string
		mimeType manifestMIMEType
	}{
		{"image.manifest.json", dockerV2Schema2MIMEType},
		{"v1s1.manifest.json", dockerV2Schema1MIMEType},
		{"v1s1-invalid-signatures.manifest.json", dockerV2Schema1MIMEType},
		{"v2s2nomime.manifest.json", dockerV2Schema2MIMEType}, // It is unclear whether this one is legal, but we should guess v2s2 if anything at all.
		{"unknown-version.manifest.json", ""},
		{"image.signature", ""}, // Not a manifest (nor JSON) at all
	}

	for _, c := range cases {
		manifest, err := ioutil.ReadFile(filepath.Join("fixtures", c.path))
		require.NoError(t, err)
		mimeType := guessManifestMIMEType(manifest)
		assert.Equal(t, c.mimeType, mimeType)
	}
}

func TestDockerManifestDigest(t *testing.T) {
	cases := []struct {
		path   string
		digest string
	}{
		{"image.manifest.json", fixtures.TestImageManifestDigest},
		{"v1s1.manifest.json", fixtures.TestV1S1ManifestDigest},
	}
	for _, c := range cases {
		manifest, err := ioutil.ReadFile(filepath.Join("fixtures", c.path))
		require.NoError(t, err)
		digest, err := dockerManifestDigest(manifest)
		require.NoError(t, err)
		assert.Equal(t, c.digest, digest)
	}

	manifest, err := ioutil.ReadFile("fixtures/v1s1-invalid-signatures.manifest.json")
	require.NoError(t, err)
	digest, err := dockerManifestDigest(manifest)
	assert.Error(t, err)

	digest, err = dockerManifestDigest([]byte{})
	require.NoError(t, err)
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

	// Error computing Docker manifest
	invalidManifest, err := ioutil.ReadFile("fixtures/v2s1-invalid-signatures.manifest.json")
	require.NoError(t, err)
	_, err = SignDockerManifest(invalidManifest, fixtures.TestImageSignatureReference, mech, fixtures.TestKeyFingerprint)
	assert.Error(t, err)

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

	// Error computing Docker manifest
	invalidManifest, err := ioutil.ReadFile("fixtures/v2s1-invalid-signatures.manifest.json")
	require.NoError(t, err)
	sig, err = VerifyDockerManifestSignature(signature, invalidManifest, fixtures.TestImageSignatureReference, mech, fixtures.TestKeyFingerprint)
	assert.Error(t, err)
	assert.Nil(t, sig)

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

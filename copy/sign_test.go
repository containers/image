package copy

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/containers/image/v5/directory"
	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/internal/imagedestination"
	internalsig "github.com/containers/image/v5/internal/signature"
	"github.com/containers/image/v5/internal/testing/gpgagent"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/types"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testGPGHomeDirectory = "../signature/fixtures"
	// TestKeyFingerprint is the fingerprint of the private key in testGPGHomeDirectory.
	// Keep this in sync with signature/fixtures_info_test.go
	testKeyFingerprint = "1D8230F6CDB6A06716E414C1DB72F2188BB46CC8"
)

// Ensure we don’t leave around GPG agent processes.
func TestMain(m *testing.M) {
	code := m.Run()
	if err := gpgagent.KillGPGAgent(testGPGHomeDirectory); err != nil {
		logrus.Warnf("Error killing GPG agent: %v", err)
	}
	os.Exit(code)
}

func TestCreateSignature(t *testing.T) {
	manifestBlob := []byte("Something")
	manifestDigest, err := manifest.Digest(manifestBlob)
	require.NoError(t, err)

	mech, _, err := signature.NewEphemeralGPGSigningMechanism([]byte{})
	require.NoError(t, err)
	defer mech.Close()
	if err := mech.SupportsSigning(); err != nil {
		t.Skipf("Signing not supported: %v", err)
	}

	t.Setenv("GNUPGHOME", testGPGHomeDirectory)

	// Signing a directory: reference, which does not have a DockerReference(), fails.
	tempDir := t.TempDir()
	dirRef, err := directory.NewReference(tempDir)
	require.NoError(t, err)
	dirDest, err := dirRef.NewImageDestination(context.Background(), nil)
	require.NoError(t, err)
	defer dirDest.Close()
	c := &copier{
		dest:         imagedestination.FromPublic(dirDest),
		reportWriter: io.Discard,
	}
	_, err = c.createSignature(manifestBlob, testKeyFingerprint, "", nil)
	assert.Error(t, err)

	// Set up a docker: reference
	dockerRef, err := docker.ParseReference("//busybox")
	require.NoError(t, err)
	dockerDest, err := dockerRef.NewImageDestination(context.Background(),
		&types.SystemContext{RegistriesDirPath: "/this/does/not/exist", DockerPerHostCertDirPath: "/this/does/not/exist"})
	require.NoError(t, err)
	defer dockerDest.Close()
	c = &copier{
		dest:         imagedestination.FromPublic(dockerDest),
		reportWriter: io.Discard,
	}

	// Signing with an unknown key fails
	_, err = c.createSignature(manifestBlob, "this key does not exist", "", nil)
	assert.Error(t, err)

	// Can't sign without a full reference
	ref, err := reference.ParseNamed("myregistry.io/myrepo")
	require.NoError(t, err)
	_, err = c.createSignature(manifestBlob, testKeyFingerprint, "", ref)
	assert.Error(t, err)

	// Mechanism for verifying the signatures
	mech, err = signature.NewGPGSigningMechanism()
	require.NoError(t, err)
	defer mech.Close()

	// Signing without overriding the identity uses the docker reference
	sig, err := c.createSignature(manifestBlob, testKeyFingerprint, "", nil)
	require.NoError(t, err)
	simpleSig, ok := sig.(internalsig.SimpleSigning)
	require.True(t, ok)
	verified, err := signature.VerifyDockerManifestSignature(simpleSig.UntrustedSignature(), manifestBlob, "docker.io/library/busybox:latest", mech, testKeyFingerprint)
	require.NoError(t, err)
	assert.Equal(t, "docker.io/library/busybox:latest", verified.DockerReference)
	assert.Equal(t, manifestDigest, verified.DockerManifestDigest)

	// Can override the identity with own
	ref, err = reference.ParseNamed("myregistry.io/myrepo:mytag")
	require.NoError(t, err)
	sig, err = c.createSignature(manifestBlob, testKeyFingerprint, "", ref)
	require.NoError(t, err)
	simpleSig, ok = sig.(internalsig.SimpleSigning)
	require.True(t, ok)
	verified, err = signature.VerifyDockerManifestSignature(simpleSig.UntrustedSignature(), manifestBlob, "myregistry.io/myrepo:mytag", mech, testKeyFingerprint)
	require.NoError(t, err)
	assert.Equal(t, "myregistry.io/myrepo:mytag", verified.DockerReference)
	assert.Equal(t, manifestDigest, verified.DockerManifestDigest)
}

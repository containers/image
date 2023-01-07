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

func TestCreateSignatures(t *testing.T) {
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

	// Set up dir: and docker: destinations
	tempDir := t.TempDir()
	dirRef, err := directory.NewReference(tempDir)
	require.NoError(t, err)
	dirDest, err := dirRef.NewImageDestination(context.Background(), nil)
	require.NoError(t, err)
	defer dirDest.Close()
	dockerRef, err := docker.ParseReference("//busybox")
	require.NoError(t, err)
	dockerDest, err := dockerRef.NewImageDestination(context.Background(),
		&types.SystemContext{RegistriesDirPath: "/this/does/not/exist", DockerPerHostCertDirPath: "/this/does/not/exist"})
	require.NoError(t, err)
	defer dockerDest.Close()

	// Mechanism for verifying the signatures
	mech, err = signature.NewGPGSigningMechanism()
	require.NoError(t, err)
	defer mech.Close()

	workingOptions := Options{SignBy: testKeyFingerprint}
	for _, cc := range []struct {
		name                       string
		dest                       types.ImageDestination
		options                    *Options
		identity                   string
		successWithNoSigs          bool
		successfullySignedIdentity string // Set to expect a successful signing with workingOptions
	}{
		{
			name:    "unknown key",
			dest:    dockerDest,
			options: &Options{SignBy: "this key does not exist"},
		},
		{
			name:     "not a full reference",
			dest:     dockerDest,
			identity: "myregistry.io/myrepo",
		},
		{
			name:              "dir: with no identity specified, but no signing request",
			dest:              dirDest,
			options:           &Options{},
			successWithNoSigs: true,
		},

		{
			name:     "dir: with no identity specified",
			dest:     dirDest,
			identity: "",
		},
		{
			name:                       "dir: with overridden identity",
			dest:                       dirDest,
			identity:                   "myregistry.io/myrepo:mytag",
			successfullySignedIdentity: "myregistry.io/myrepo:mytag",
		},
		{
			name:                       "docker:// without overriding the identity",
			dest:                       dockerDest,
			identity:                   "",
			successfullySignedIdentity: "docker.io/library/busybox:latest",
		},
		{
			name:                       "docker:// with overidden identity",
			dest:                       dockerDest,
			identity:                   "myregistry.io/myrepo:mytag",
			successfullySignedIdentity: "myregistry.io/myrepo:mytag",
		},
	} {
		var identity reference.Named = nil
		if cc.identity != "" {
			i, err := reference.ParseNormalizedNamed(cc.identity)
			require.NoError(t, err, cc.name)
			identity = i
		}
		options := cc.options
		if options == nil {
			options = &workingOptions
		}

		c := &copier{
			dest:         imagedestination.FromPublic(cc.dest),
			reportWriter: io.Discard,
		}
		defer c.close()
		err := c.setupSigners(options)
		require.NoError(t, err, cc.name)
		sigs, err := c.createSignatures(context.Background(), manifestBlob, identity)
		switch {
		case cc.successfullySignedIdentity != "":
			require.NoError(t, err, cc.name)
			require.Len(t, sigs, 1, cc.name)
			simpleSig, ok := sigs[0].(internalsig.SimpleSigning)
			require.True(t, ok, cc.name)
			verified, err := signature.VerifyDockerManifestSignature(simpleSig.UntrustedSignature(), manifestBlob, cc.successfullySignedIdentity, mech, workingOptions.SignBy)
			require.NoError(t, err, cc.name)
			assert.Equal(t, cc.successfullySignedIdentity, verified.DockerReference, cc.name)
			assert.Equal(t, manifestDigest, verified.DockerManifestDigest, cc.name)

		case cc.successWithNoSigs:
			require.NoError(t, err, cc.name)
			require.Empty(t, sigs, cc.name)

		default:
			assert.Error(t, err, cc.name)
		}
	}
}

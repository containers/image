package simplesigning

import (
	"context"
	"os"
	"testing"

	"github.com/containers/image/v5/docker/reference"
	internalSig "github.com/containers/image/v5/internal/signature"
	internalSigner "github.com/containers/image/v5/internal/signer"
	"github.com/containers/image/v5/internal/testing/gpgagent"
	"github.com/containers/image/v5/signature"
	"github.com/opencontainers/go-digest"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// testImageManifestDigest is the Docker manifest digest of "image.manifest.json"
	testImageManifestDigest = digest.Digest("sha256:20bf21ed457b390829cdbeec8795a7bea1626991fda603e0d01b4e7f60427e55")
	testGPGHomeDirectory    = "./testdata"
	// TestKeyFingerprint is the fingerprint of the private key in testGPGHomeDirectory.
	testKeyFingerprint = "1D8230F6CDB6A06716E414C1DB72F2188BB46CC8"
	// testKeyFingerprintWithPassphrase is the fingerprint of the private key with passphrase in testGPGHomeDirectory.
	testKeyFingerprintWithPassphrase = "E3EB7611D815211F141946B5B0CDE60B42557346"
	// testPassphrase is the passphrase for testKeyFingerprintWithPassphrase.
	testPassphrase = "WithPassphrase123"
)

// Ensure we don’t leave around GPG agent processes.
func TestMain(m *testing.M) {
	code := m.Run()
	if err := gpgagent.KillGPGAgent(testGPGHomeDirectory); err != nil {
		logrus.Warnf("Error killing GPG agent: %v", err)
	}
	os.Exit(code)
}

func TestNewSigner(t *testing.T) {
	t.Setenv("GNUPGHOME", testGPGHomeDirectory)

	mech, err := signature.NewGPGSigningMechanism()
	require.NoError(t, err)
	defer mech.Close()
	if err := mech.SupportsSigning(); err != nil {
		t.Skipf("Signing not supported: %v", err)
	}

	// An option causes an error
	_, err = NewSigner(WithKeyFingerprint(testKeyFingerprintWithPassphrase), WithPassphrase("\n"))
	assert.Error(t, err)

	// WithKeyFingerprint is missing
	_, err = NewSigner(WithPassphrase("something"))
	assert.Error(t, err)

	// A smoke test
	s, err := NewSigner(WithKeyFingerprint(testKeyFingerprint))
	require.NoError(t, err)
	err = s.Close()
	assert.NoError(t, err)
}

func TestSimpleSignerProgressMessage(t *testing.T) {
	t.Setenv("GNUPGHOME", testGPGHomeDirectory)

	mech, err := signature.NewGPGSigningMechanism()
	require.NoError(t, err)
	defer mech.Close()
	if err := mech.SupportsSigning(); err != nil {
		t.Skipf("Signing not supported: %v", err)
	}

	// Just a smoke test
	s, err := NewSigner(WithKeyFingerprint(testKeyFingerprint))
	require.NoError(t, err)
	defer func() {
		err = s.Close()
		assert.NoError(t, err)
	}()

	_ = internalSigner.ProgressMessage(s)
}

func TestSimpleSignerSignImageManifest(t *testing.T) {
	t.Setenv("GNUPGHOME", testGPGHomeDirectory)

	mech, err := signature.NewGPGSigningMechanism()
	require.NoError(t, err)
	defer mech.Close()
	if err := mech.SupportsSigning(); err != nil {
		t.Skipf("Signing not supported: %v", err)
	}

	err = gpgagent.KillGPGAgent(testGPGHomeDirectory)
	require.NoError(t, err)

	manifest, err := os.ReadFile("../fixtures/image.manifest.json")
	require.NoError(t, err)
	testImageSignatureReference, err := reference.ParseNormalizedNamed("example.com/testing/manifest:notlatest")
	require.NoError(t, err)

	// Failures to sign need to be tested in two parts: First the failures that involve the wrong passphrase, then failures that
	// should manifest even with a valid passphrase or unlocked key (because the GPG agent is caching unlocked keys).
	// Alternatively, we could be calling gpgagent.KillGPGAgent() all the time...
	type failingCase struct {
		name string
		opts []Option
		// NOTE: We DO NOT promise that things that don't fail during NewSigner won't start failing there.
		// Actually we’d prefer failures to be identified early. This field only records current expected behavior, not the _desired_ end state.
		creationFails         bool
		creationErrorContains string
		manifest              []byte
		ref                   reference.Named
	}
	testFailure := func(c failingCase) {
		s, err := NewSigner(c.opts...)
		if c.creationFails {
			assert.Error(t, err, c.name)
			if c.creationErrorContains != "" {
				assert.ErrorContains(t, err, c.creationErrorContains, c.name)
			}
		} else {
			require.NoError(t, err, c.name)
			defer s.Close()

			m := manifest
			if c.manifest != nil {
				m = c.manifest
			}
			_, err = internalSigner.SignImageManifest(context.Background(), s, m, c.ref)
			assert.Error(t, err, c.name)
		}
	}
	for _, c := range []failingCase{
		{
			name: "Invalid passphrase",
			opts: []Option{
				WithKeyFingerprint(testKeyFingerprintWithPassphrase),
				WithPassphrase(testPassphrase + "\n"),
			},
			creationFails:         true,
			creationErrorContains: "invalid passphrase",
			ref:                   testImageSignatureReference,
		},
		{
			name: "Wrong passphrase",
			opts: []Option{
				WithKeyFingerprint(testKeyFingerprintWithPassphrase),
				WithPassphrase("wrong"),
			},
			ref: testImageSignatureReference,
		},
		{
			name: "No passphrase",
			opts: []Option{WithKeyFingerprint(testKeyFingerprintWithPassphrase)},
			ref:  testImageSignatureReference,
		},
	} {
		testFailure(c)
	}

	// Successful signing
	for _, c := range []struct {
		name        string
		fingerprint string
		opts        []Option
	}{
		{
			name:        "No passphrase",
			fingerprint: testKeyFingerprint,
		},
		{
			name:        "With passphrase",
			fingerprint: testKeyFingerprintWithPassphrase,
			opts:        []Option{WithPassphrase(testPassphrase)},
		},
	} {
		s, err := NewSigner(append([]Option{WithKeyFingerprint(c.fingerprint)}, c.opts...)...)
		require.NoError(t, err, c.name)
		defer s.Close()

		sig, err := internalSigner.SignImageManifest(context.Background(), s, manifest, testImageSignatureReference)
		require.NoError(t, err, c.name)
		simpleSig, ok := sig.(internalSig.SimpleSigning)
		require.True(t, ok)

		// FIXME FIXME: gpgme_op_sign with a passphrase succeeds, but somehow confuses the GPGME internal state
		// so that gpgme_op_verify below never completes (it polls on an already closed FD).
		// That’s probably a GPGME bug, and needs investigating and fixing, but it isn’t related to this “signer” implementation.
		if len(c.opts) == 0 {
			mech, err := signature.NewGPGSigningMechanism()
			require.NoError(t, err)
			defer mech.Close()

			verified, err := signature.VerifyDockerManifestSignature(simpleSig.UntrustedSignature(), manifest, testImageSignatureReference.String(), mech, c.fingerprint)
			require.NoError(t, err)
			assert.Equal(t, testImageSignatureReference.String(), verified.DockerReference)
			assert.Equal(t, testImageManifestDigest, verified.DockerManifestDigest)
		}
	}

	invalidManifest, err := os.ReadFile("../fixtures/v2s1-invalid-signatures.manifest.json")
	require.NoError(t, err)
	invalidReference, err := reference.ParseNormalizedNamed("no-tag")
	require.NoError(t, err)
	for _, c := range []failingCase{
		{
			name:          "No key to sign with",
			opts:          nil,
			creationFails: true,
		},
		{
			name:     "Error computing Docker manifest",
			opts:     []Option{WithKeyFingerprint(testKeyFingerprint)},
			manifest: invalidManifest,
			ref:      testImageSignatureReference,
		},
		{
			name: "Invalid reference",
			opts: []Option{WithKeyFingerprint(testKeyFingerprint)},
			ref:  invalidReference,
		},
		{
			name: "Error signing",
			opts: []Option{
				WithKeyFingerprint("this fingerprint doesn't exist"),
			},
			ref: testImageSignatureReference,
		},
	} {
		testFailure(c)
	}
}

//go:build containers_image_sequoia

package simplesequoia

import (
	"context"
	"os"
	"testing"

	"github.com/containers/image/v5/docker/reference"
	internalSig "github.com/containers/image/v5/internal/signature"
	internalSigner "github.com/containers/image/v5/internal/signer"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/signature/internal/sequoia/testcli"
	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// testImageManifestDigest is the Docker manifest digest of "image.manifest.json"
	testImageManifestDigest = digest.Digest("sha256:20bf21ed457b390829cdbeec8795a7bea1626991fda603e0d01b4e7f60427e55")
	// FIXME
	// testGPGHomeDirectory    = "./testdata"
	// // testKeyFingerprint is the fingerprint of the private key in testGPGHomeDirectory.
	// testKeyFingerprint = "08CD26E446E2E95249B7A405E932F44B23E8DD43"
	// // testKeyFingerprintWithPassphrase is the fingerprint of the private key with passphrase in testGPGHomeDirectory.
	// testKeyFingerprintWithPassphrase = "F2B501009F78B0B340221A12A3CD242DA6028093"
	// // testPassphrase is the passphrase for testKeyFingerprintWithPassphrase.
	// testPassphrase = "WithPassphrase123"
)

func TestNewSigner(t *testing.T) {
	if err := testcli.CheckCliVersion("1.3.0"); err != nil {
		t.Skipf("sq not usable: %v", err)
	}

	homeDir := t.TempDir() // FIXME: Also (only?) test a pre-existing fixture
	fingerprint, err := testcli.GenerateKey(homeDir, "test@email")
	require.NoError(t, err)

	// An option causes an error
	_, err = NewSigner(WithSequoiaHome(homeDir), WithKeyFingerprint(fingerprint), WithPassphrase("\n"))
	assert.Error(t, err)

	// WithSequoiaHome is missing
	_, err = NewSigner(WithKeyFingerprint(fingerprint), WithPassphrase("something"))
	assert.Error(t, err)

	// WithKeyFingerprint is missing
	_, err = NewSigner(WithSequoiaHome(homeDir), WithPassphrase("something"))
	assert.Error(t, err)

	// A smoke test
	s, err := NewSigner(WithSequoiaHome(homeDir), WithKeyFingerprint(fingerprint))
	require.NoError(t, err)
	err = s.Close()
	assert.NoError(t, err)
}

func TestSimpleSignerProgressMessage(t *testing.T) {
	if err := testcli.CheckCliVersion("1.3.0"); err != nil {
		t.Skipf("sq not usable: %v", err)
	}

	homeDir := t.TempDir() // FIXME: Also (only?) test a pre-existing fixture

	// Just a smoke test
	s, err := NewSigner(WithSequoiaHome(homeDir), WithKeyFingerprint("not used"))
	require.NoError(t, err)
	defer func() {
		err = s.Close()
		assert.NoError(t, err)
	}()

	_ = internalSigner.ProgressMessage(s)
}

func TestSimpleSignerSignImageManifest(t *testing.T) {
	if err := testcli.CheckCliVersion("1.3.0"); err != nil {
		t.Skipf("sq not usable: %v", err)
	}

	homeDir := t.TempDir() // FIXME: Also (only?) test a pre-existing fixture
	fingerprint, err := testcli.GenerateKey(homeDir, "test@email")
	require.NoError(t, err)
	publicKey, err := testcli.ExportCert(homeDir, fingerprint)
	require.NoError(t, err)

	manifest, err := os.ReadFile("../fixtures/image.manifest.json")
	require.NoError(t, err)
	testImageSignatureReference, err := reference.ParseNormalizedNamed("example.com/testing/manifest:notlatest")
	require.NoError(t, err)

	// Failures to sign need to be tested in two parts: First the failures that involve the wrong passphrase, then failures that
	// should manifest even with a valid passphrase or unlocked key (because the GPG agent is caching unlocked keys).
	// FIXME: Is this still relevant with Sequoia???
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
		// {// FIXME
		// 	name: "Invalid passphrase",
		// 	opts: []Option{
		// 		WithSequoiaHome(homeDir),
		// 		WithKeyFingerprint(testKeyFingerprintWithPassphrase),
		// 		WithPassphrase(testPassphrase + "\n"),
		// 	},
		// 	creationFails:         true,
		// 	creationErrorContains: "invalid passphrase",
		// 	ref:                   testImageSignatureReference,
		// },
		// {
		// 	name: "Wrong passphrase",
		// 	opts: []Option{
		// 		WithSequoiaHome(homeDir),
		// 		WithKeyFingerprint(testKeyFingerprintWithPassphrase),
		// 		WithPassphrase("wrong"),
		// 	},
		// 	ref: testImageSignatureReference,
		// },
		// {
		// 	name: "No passphrase",
		// 	opts: []Option{WithKeyFingerprint(testKeyFingerprintWithPassphrase)},
		// 	ref:  testImageSignatureReference,
		// },
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
			fingerprint: fingerprint,
		},
		// { // FIXME
		// 	name:        "With passphrase",
		// 	fingerprint: testKeyFingerprintWithPassphrase,
		// 	opts:        []Option{WithPassphrase(testPassphrase)},
		// },
	} {
		s, err := NewSigner(append([]Option{WithSequoiaHome(homeDir), WithKeyFingerprint(c.fingerprint)}, c.opts...)...)
		require.NoError(t, err, c.name)
		defer s.Close()

		sig, err := internalSigner.SignImageManifest(context.Background(), s, manifest, testImageSignatureReference)
		require.NoError(t, err, c.name)
		simpleSig, ok := sig.(internalSig.SimpleSigning)
		require.True(t, ok)

		mech, importedFingerprint, err := signature.NewEphemeralGPGSigningMechanism(publicKey)
		require.NoError(t, err)
		assert.Equal(t, []string{c.fingerprint}, importedFingerprint)
		defer mech.Close()

		verified, err := signature.VerifyDockerManifestSignature(simpleSig.UntrustedSignature(), manifest, testImageSignatureReference.String(), mech, c.fingerprint)
		require.NoError(t, err)
		assert.Equal(t, testImageSignatureReference.String(), verified.DockerReference)
		assert.Equal(t, testImageManifestDigest, verified.DockerManifestDigest)
	}

	invalidManifest, err := os.ReadFile("../fixtures/v2s1-invalid-signatures.manifest.json")
	require.NoError(t, err)
	invalidReference, err := reference.ParseNormalizedNamed("no-tag")
	require.NoError(t, err)
	for _, c := range []failingCase{
		{
			name:          "No key to sign with",
			opts:          []Option{WithSequoiaHome(homeDir)},
			creationFails: true,
		},
		{
			name: "Error computing Docker manifest",
			opts: []Option{
				WithSequoiaHome(homeDir),
				WithKeyFingerprint(fingerprint),
			},
			manifest: invalidManifest,
			ref:      testImageSignatureReference,
		},
		{
			name: "Invalid reference",
			opts: []Option{
				WithSequoiaHome(homeDir),
				WithKeyFingerprint(fingerprint),
			},
			ref: invalidReference,
		},
		{
			name: "Error signing",
			opts: []Option{
				WithSequoiaHome(homeDir),
				WithKeyFingerprint("this fingerprint doesn't exist"),
			},
			ref: testImageSignatureReference,
		},
	} {
		testFailure(c)
	}
}

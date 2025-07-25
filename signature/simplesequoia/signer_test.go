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
	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// testImageManifestDigest is the Docker manifest digest of "image.manifest.json"
	testImageManifestDigest = digest.Digest("sha256:20bf21ed457b390829cdbeec8795a7bea1626991fda603e0d01b4e7f60427e55")

	testSequoiaHome = "./testdata"
	// testKeyFingerprint is a fingerprint of a test key in testSequoiaHome, generated using
	// > sq --home $(pwd)/signature/simplesequoia/testdata key generate --name 'Skopeo Sequoia testing key' --own-key --expiration=never
	testKeyFingerprint = "50DDE898DF4E48755C8C2B7AF6F908B6FA48A229"
	// testKeyFingerprintWithPassphrase  is a fingerprint of a test key in testSequoiaHome, generated using
	// > sq --home $(pwd)/signature/simplesequoia/testdata key generate --name 'Skopeo Sequoia testing key with passphrase' --own-key --expiration=never
	testKeyFingerprintWithPassphrase = "1F5825285B785E1DB13BF36D2D11A19ABA41C6AE"
	// testPassphrase is the passphrase for testKeyFingerprintWithPassphrase.
	testPassphrase = "WithPassphrase123"
)

func TestNewSigner(t *testing.T) {
	// An option causes an error
	_, err := NewSigner(WithSequoiaHome(testSequoiaHome), WithKeyFingerprint(testKeyFingerprint), WithPassphrase("\n"))
	assert.Error(t, err)

	// WithKeyFingerprint is missing
	_, err = NewSigner(WithSequoiaHome(testSequoiaHome), WithPassphrase("something"))
	assert.Error(t, err)

	// A smoke test
	s, err := NewSigner(WithSequoiaHome(testSequoiaHome), WithKeyFingerprint(testKeyFingerprint))
	require.NoError(t, err)
	err = s.Close()
	assert.NoError(t, err)

	t.Setenv("SEQUOIA_CRYPTO_POLICY", "this/does/not/exist") // Both unreadable files, and relative paths, should cause an error.
	_, err = NewSigner(WithSequoiaHome(testSequoiaHome), WithKeyFingerprint(testKeyFingerprint))
	assert.Error(t, err)
}

func TestSimpleSignerProgressMessage(t *testing.T) {
	// Just a smoke test
	s, err := NewSigner(WithSequoiaHome(testSequoiaHome), WithKeyFingerprint(testKeyFingerprint))
	require.NoError(t, err)
	defer func() {
		err = s.Close()
		assert.NoError(t, err)
	}()

	_ = internalSigner.ProgressMessage(s)
}

func TestSimpleSignerSignImageManifest(t *testing.T) {
	manifest, err := os.ReadFile("../fixtures/image.manifest.json")
	require.NoError(t, err)
	testImageSignatureReference, err := reference.ParseNormalizedNamed("example.com/testing/manifest:notlatest")
	require.NoError(t, err)

	// Successful signing
	for _, c := range []struct {
		name          string
		publicKeyPath string
		fingerprint   string
		opts          []Option
	}{
		{
			name:          "No passphrase",
			publicKeyPath: "./testdata/no-passphrase.pub",
			fingerprint:   testKeyFingerprint,
		},
		{
			name:          "With passphrase",
			publicKeyPath: "./testdata/with-passphrase.pub",
			fingerprint:   testKeyFingerprintWithPassphrase,
			opts:          []Option{WithPassphrase(testPassphrase)},
		},
	} {
		s, err := NewSigner(append([]Option{WithSequoiaHome(testSequoiaHome), WithKeyFingerprint(c.fingerprint)}, c.opts...)...)
		require.NoError(t, err, c.name)
		defer s.Close()

		sig, err := internalSigner.SignImageManifest(context.Background(), s, manifest, testImageSignatureReference)
		require.NoError(t, err, c.name)
		simpleSig, ok := sig.(internalSig.SimpleSigning)
		require.True(t, ok)

		publicKey, err := os.ReadFile(c.publicKeyPath)
		require.NoError(t, err)
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
	for _, c := range []struct {
		name string
		opts []Option
		// NOTE: We DO NOT promise that things that don't fail during NewSigner won't start failing there.
		// Actually we’d prefer failures to be identified early. This field only records current expected behavior, not the _desired_ end state.
		creationFails         bool
		creationErrorContains string
		manifest              []byte
		ref                   reference.Named
	}{
		{
			name:          "No key to sign with",
			opts:          []Option{WithSequoiaHome(testSequoiaHome)},
			creationFails: true,
		},
		{
			name: "Invalid passphrase",
			opts: []Option{
				WithSequoiaHome(testSequoiaHome),
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
				WithSequoiaHome(testSequoiaHome),
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
		{
			name: "Error computing Docker manifest",
			opts: []Option{
				WithSequoiaHome(testSequoiaHome),
				WithKeyFingerprint(testKeyFingerprint),
			},
			manifest: invalidManifest,
			ref:      testImageSignatureReference,
		},
		{
			name: "Invalid reference",
			opts: []Option{
				WithSequoiaHome(testSequoiaHome),
				WithKeyFingerprint(testKeyFingerprint),
			},
			ref: invalidReference,
		},
		{
			name: "Error signing",
			opts: []Option{
				WithSequoiaHome(testSequoiaHome),
				WithKeyFingerprint("this fingerprint doesn't exist"),
			},
			ref: testImageSignatureReference,
		},
	} {
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
}

package sigstore

import (
	"context"
	"crypto"
	"os"
	"path/filepath"
	"testing"

	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/internal/signature"
	internalSigner "github.com/containers/image/v5/internal/signer"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/signature/internal"
	"github.com/opencontainers/go-digest"
	"github.com/sigstore/sigstore/pkg/cryptoutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateKeyPair(t *testing.T) {
	// Test that generation is possible, and the key can be used for signing.
	testManifest := []byte("{}")
	testDockerReference, err := reference.ParseNormalizedNamed("example.com/foo:notlatest")
	require.NoError(t, err)

	passphrase := []byte("some passphrase")
	keyPair, err := GenerateKeyPair(passphrase)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	privateKeyFile := filepath.Join(tmpDir, "private.key")
	err = os.WriteFile(privateKeyFile, keyPair.PrivateKey, 0600)
	require.NoError(t, err)

	signer, err := NewSigner(WithPrivateKeyFile(privateKeyFile, passphrase))
	require.NoError(t, err)
	sig0, err := internalSigner.SignImageManifest(context.Background(), signer, testManifest, testDockerReference)
	require.NoError(t, err)
	sig, ok := sig0.(signature.Sigstore)
	require.True(t, ok)

	// It would be even more elegant to invoke the higher-level prSigstoreSigned code,
	// but that is private.
	publicKey, err := cryptoutils.UnmarshalPEMToPublicKey(keyPair.PublicKey)
	require.NoError(t, err)
	publicKeys := []crypto.PublicKey{publicKey}

	_, _, err = internal.VerifySigstorePayload(publicKeys, sig.UntrustedPayload(),
		sig.UntrustedAnnotations()[signature.SigstoreSignatureAnnotationKey],
		internal.SigstorePayloadAcceptanceRules{
			ValidateSignedDockerReference: func(ref string) error {
				assert.Equal(t, "example.com/foo:notlatest", ref)
				return nil
			},
			ValidateSignedDockerManifestDigest: func(digest digest.Digest) error {
				matches, err := manifest.MatchesDigest(testManifest, digest)
				require.NoError(t, err)
				assert.True(t, matches)
				return nil
			},
		})
	assert.NoError(t, err)

	// The failure paths are not obviously easy to reach.
}

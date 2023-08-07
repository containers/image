package copy

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/containers/image/v5/directory"
	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/internal/imagedestination"
	internalsig "github.com/containers/image/v5/internal/signature"
	internalSigner "github.com/containers/image/v5/internal/signer"
	"github.com/containers/image/v5/signature/signer"
	"github.com/containers/image/v5/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubSignerImpl is a signer.SigningImplementation that allows us to check the signed identity, without the overhead of actually signing.
// We abuse internalsig.Sigstore to store the signed manifest and identity in the payload and MIME type fields, respectively.
type stubSignerImpl struct {
	signingFailure error // if set, SignImageManifest returns this
}

func (s *stubSignerImpl) ProgressMessage() string {
	return "Signing with stubSigner"
}

func (s *stubSignerImpl) SignImageManifest(ctx context.Context, m []byte, dockerReference reference.Named) (internalsig.Signature, error) {
	if s.signingFailure != nil {
		return nil, s.signingFailure
	}
	return internalsig.SigstoreFromComponents(dockerReference.String(), m, nil), nil
}

func (s *stubSignerImpl) Close() error {
	return nil
}

func TestCreateSignatures(t *testing.T) {
	stubSigner := internalSigner.NewSigner(&stubSignerImpl{})
	defer stubSigner.Close()

	manifestBlob := []byte("Something")
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

	workingOptions := Options{Signers: []*signer.Signer{stubSigner}}
	for _, cc := range []struct {
		name                       string
		dest                       types.ImageDestination
		options                    *Options
		identity                   string
		successWithNoSigs          bool
		successfullySignedIdentity string // Set to expect a successful signing with workingOptions
	}{
		{
			name: "signing fails",
			dest: dockerDest,
			options: &Options{
				Signers: []*signer.Signer{
					internalSigner.NewSigner(&stubSignerImpl{signingFailure: errors.New("fails")}),
				},
			},
		},
		{
			name: "second signing fails",
			dest: dockerDest,
			options: &Options{
				Signers: []*signer.Signer{
					stubSigner,
					internalSigner.NewSigner(&stubSignerImpl{signingFailure: errors.New("fails")}),
				},
			},
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
			options:      options,
			reportWriter: io.Discard,
		}
		defer c.close()
		err := c.setupSigners()
		require.NoError(t, err, cc.name)
		sigs, err := c.createSignatures(context.Background(), manifestBlob, identity)
		switch {
		case cc.successfullySignedIdentity != "":
			require.NoError(t, err, cc.name)
			require.Len(t, sigs, 1, cc.name)
			stubSig, ok := sigs[0].(internalsig.Sigstore)
			require.True(t, ok, cc.name)
			// Compare how stubSignerImpl.SignImageManifest stuffs the signing parameters into these fields.
			assert.Equal(t, manifestBlob, stubSig.UntrustedPayload(), cc.name)
			assert.Equal(t, cc.successfullySignedIdentity, stubSig.UntrustedMIMEType(), cc.name)

		case cc.successWithNoSigs:
			require.NoError(t, err, cc.name)
			require.Empty(t, sigs, cc.name)

		default:
			assert.Error(t, err, cc.name)
		}
	}
}

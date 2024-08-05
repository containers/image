package signature

import (
	"context"
	"os"
	"path"
	"testing"

	"github.com/containers/image/v5/directory"
	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/internal/image"
	"github.com/containers/image/v5/internal/imagesource"
	"github.com/containers/image/v5/internal/private"
	"github.com/containers/image/v5/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dirImageMock returns a private.UnparsedImage for a directory, claiming a specified dockerReference.
func dirImageMock(t *testing.T, dir, dockerReference string) private.UnparsedImage {
	ref, err := reference.ParseNormalizedNamed(dockerReference)
	require.NoError(t, err)
	return dirImageMockWithRef(t, dir, refImageReferenceMock{ref: ref})
}

// dirImageMockWithRef returns a private.UnparsedImage for a directory, claiming a specified ref.
func dirImageMockWithRef(t *testing.T, dir string, ref types.ImageReference) private.UnparsedImage {
	srcRef, err := directory.NewReference(dir)
	require.NoError(t, err)
	src, err := srcRef.NewImageSource(context.Background(), nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		err := src.Close()
		require.NoError(t, err)
	})
	return image.UnparsedInstance(&dirImageSourceMock{
		ImageSource: imagesource.FromPublic(src),
		ref:         ref,
	}, nil)
}

// dirImageSourceMock inherits dirImageSource, but overrides its Reference method.
type dirImageSourceMock struct {
	private.ImageSource
	ref types.ImageReference
}

func (d *dirImageSourceMock) Reference() types.ImageReference {
	return d.ref
}

func TestPRSignedByIsSignatureAuthorAccepted(t *testing.T) {
	ktGPG := SBKeyTypeGPGKeys
	prm := NewPRMMatchExact()
	testImage := dirImageMock(t, "fixtures/dir-img-valid", "testing/manifest:latest")
	testImageSig, err := os.ReadFile("fixtures/dir-img-valid/signature-1")
	require.NoError(t, err)
	keyData, err := os.ReadFile("fixtures/public-key.gpg")
	require.NoError(t, err)

	// Successful validation, with KeyPath, KeyPaths and KeyData.
	for _, fn := range []func() (PolicyRequirement, error){
		func() (PolicyRequirement, error) {
			return NewPRSignedByKeyPath(ktGPG, "fixtures/public-key.gpg", prm)
		},
		// Test the files in both orders, to make sure the correct public keys accepted in either position.
		func() (PolicyRequirement, error) {
			return NewPRSignedByKeyPaths(ktGPG, []string{"fixtures/public-key-1.gpg", "fixtures/public-key-1.gpg"}, prm)
		},
		func() (PolicyRequirement, error) {
			return NewPRSignedByKeyPaths(ktGPG, []string{"fixtures/public-key-2.gpg", "fixtures/public-key-1.gpg"}, prm)
		},
		func() (PolicyRequirement, error) {
			return NewPRSignedByKeyData(ktGPG, keyData, prm)
		},
	} {
		pr, err := fn()
		require.NoError(t, err)
		sar, parsedSig, err := pr.isSignatureAuthorAccepted(context.Background(), testImage, testImageSig)
		assertSARAccepted(t, sar, parsedSig, err, Signature{
			DockerManifestDigest: TestImageManifestDigest,
			DockerReference:      "testing/manifest:latest",
		})
	}

	// Unimplemented and invalid KeyType values
	for _, keyType := range []sbKeyType{SBKeyTypeSignedByGPGKeys,
		SBKeyTypeX509Certificates,
		SBKeyTypeSignedByX509CAs,
		sbKeyType("This is invalid"),
	} {
		// Do not use NewPRSignedByKeyData, because it would reject invalid values.
		pr := &prSignedBy{
			KeyType:        keyType,
			KeyData:        keyData,
			SignedIdentity: prm,
		}
		// Pass nil pointers to, kind of, test that the return value does not depend on the parameters.
		sar, parsedSig, err := pr.isSignatureAuthorAccepted(context.Background(), nil, nil)
		assertSARRejected(t, sar, parsedSig, err)
	}

	// Invalid KeyPath/KeyPaths/KeyData combinations.
	for _, fn := range []func() (PolicyRequirement, error){
		// Two or more of KeyPath, KeyPaths and KeyData set. Do not use NewPRSignedBy*, because it would reject this.
		func() (PolicyRequirement, error) {
			return &prSignedBy{KeyType: ktGPG, KeyPath: "fixtures/public-key.gpg", KeyPaths: []string{"fixtures/public-key-1.gpg", "fixtures/public-key-2.gpg"}, KeyData: keyData, SignedIdentity: prm}, nil
		},
		func() (PolicyRequirement, error) {
			return &prSignedBy{KeyType: ktGPG, KeyPath: "fixtures/public-key.gpg", KeyPaths: []string{"fixtures/public-key-1.gpg", "fixtures/public-key-2.gpg"}, SignedIdentity: prm}, nil
		},
		func() (PolicyRequirement, error) {
			return &prSignedBy{KeyType: ktGPG, KeyPath: "fixtures/public-key.gpg", KeyData: keyData, SignedIdentity: prm}, nil
		},
		func() (PolicyRequirement, error) {
			return &prSignedBy{KeyType: ktGPG, KeyPaths: []string{"fixtures/public-key-1.gpg", "fixtures/public-key-2.gpg"}, KeyData: keyData, SignedIdentity: prm}, nil
		},
		// None of KeyPath, KeyPaths and KeyData set. Do not use NewPRSignedBy*, because it would reject this.
		func() (PolicyRequirement, error) {
			return &prSignedBy{KeyType: ktGPG, SignedIdentity: prm}, nil
		},
		func() (PolicyRequirement, error) { // Invalid KeyPath
			return NewPRSignedByKeyPath(ktGPG, "/this/does/not/exist", prm)
		},
		func() (PolicyRequirement, error) { // Invalid KeyPaths
			return NewPRSignedByKeyPaths(ktGPG, []string{"/this/does/not/exist"}, prm)
		},
		func() (PolicyRequirement, error) { // One of the KeyPaths is invalid
			return NewPRSignedByKeyPaths(ktGPG, []string{"fixtures/public-key.gpg", "/this/does/not/exist"}, prm)
		},
	} {
		pr, err := fn()
		require.NoError(t, err)
		// Pass nil pointers to, kind of, test that the return value does not depend on the parameters.
		sar, parsedSig, err := pr.isSignatureAuthorAccepted(context.Background(), nil, nil)
		assertSARRejected(t, sar, parsedSig, err)
	}

	// Errors initializing the temporary GPG directory and mechanism are not obviously easy to reach.

	// KeyData has no public keys.
	pr, err := NewPRSignedByKeyData(ktGPG, []byte{}, prm)
	require.NoError(t, err)
	// Pass nil pointers to, kind of, test that the return value does not depend on the parameters.
	sar, parsedSig, err := pr.isSignatureAuthorAccepted(context.Background(), nil, nil)
	assertSARRejectedPolicyRequirement(t, sar, parsedSig, err)

	// A signature which does not GPG verify
	pr, err = NewPRSignedByKeyPath(ktGPG, "fixtures/public-key.gpg", prm)
	require.NoError(t, err)
	// Pass a nil pointer to, kind of, test that the return value does not depend on the image parameter.
	sar, parsedSig, err = pr.isSignatureAuthorAccepted(context.Background(), nil, []byte("invalid signature"))
	assertSARRejected(t, sar, parsedSig, err)

	// A valid signature using an unknown key.
	// (This is (currently?) rejected through the "mech.Verify fails" path, not the "!identityFound" path,
	// because we use a temporary directory and only import the trusted keys.)
	pr, err = NewPRSignedByKeyPath(ktGPG, "fixtures/public-key.gpg", prm)
	require.NoError(t, err)
	sig, err := os.ReadFile("fixtures/unknown-key.signature")
	require.NoError(t, err)
	// Pass a nil pointer to, kind of, test that the return value does not depend on the image parameter..
	sar, parsedSig, err = pr.isSignatureAuthorAccepted(context.Background(), nil, sig)
	assertSARRejected(t, sar, parsedSig, err)

	// A valid signature of an invalid JSON.
	pr, err = NewPRSignedByKeyPath(ktGPG, "fixtures/public-key.gpg", prm)
	require.NoError(t, err)
	sig, err = os.ReadFile("fixtures/invalid-blob.signature")
	require.NoError(t, err)
	// Pass a nil pointer to, kind of, test that the return value does not depend on the image parameter..
	sar, parsedSig, err = pr.isSignatureAuthorAccepted(context.Background(), nil, sig)
	assertSARRejected(t, sar, parsedSig, err)
	assert.IsType(t, InvalidSignatureError{}, err)

	// A valid signature with a rejected identity.
	nonmatchingPRM, err := NewPRMExactReference("this/does-not:match")
	require.NoError(t, err)
	pr, err = NewPRSignedByKeyPath(ktGPG, "fixtures/public-key.gpg", nonmatchingPRM)
	require.NoError(t, err)
	sar, parsedSig, err = pr.isSignatureAuthorAccepted(context.Background(), testImage, testImageSig)
	assertSARRejectedPolicyRequirement(t, sar, parsedSig, err)

	// Error reading image manifest
	image := dirImageMock(t, "fixtures/dir-img-no-manifest", "testing/manifest:latest")
	sig, err = os.ReadFile("fixtures/dir-img-no-manifest/signature-1")
	require.NoError(t, err)
	pr, err = NewPRSignedByKeyPath(ktGPG, "fixtures/public-key.gpg", prm)
	require.NoError(t, err)
	sar, parsedSig, err = pr.isSignatureAuthorAccepted(context.Background(), image, sig)
	assertSARRejected(t, sar, parsedSig, err)

	// Error computing manifest digest
	image = dirImageMock(t, "fixtures/dir-img-manifest-digest-error", "testing/manifest:latest")
	sig, err = os.ReadFile("fixtures/dir-img-manifest-digest-error/signature-1")
	require.NoError(t, err)
	pr, err = NewPRSignedByKeyPath(ktGPG, "fixtures/public-key.gpg", prm)
	require.NoError(t, err)
	sar, parsedSig, err = pr.isSignatureAuthorAccepted(context.Background(), image, sig)
	assertSARRejected(t, sar, parsedSig, err)

	// A valid signature with a non-matching manifest
	image = dirImageMock(t, "fixtures/dir-img-modified-manifest", "testing/manifest:latest")
	sig, err = os.ReadFile("fixtures/dir-img-modified-manifest/signature-1")
	require.NoError(t, err)
	pr, err = NewPRSignedByKeyPath(ktGPG, "fixtures/public-key.gpg", prm)
	require.NoError(t, err)
	sar, parsedSig, err = pr.isSignatureAuthorAccepted(context.Background(), image, sig)
	assertSARRejectedPolicyRequirement(t, sar, parsedSig, err)
}

// createInvalidSigDir creates a directory suitable for dirImageMock, in which image.Signatures()
// fails.
func createInvalidSigDir(t *testing.T) string {
	dir := t.TempDir()
	err := os.WriteFile(path.Join(dir, "manifest.json"), []byte("{}"), 0644)
	require.NoError(t, err)
	// Creating a 000-permissions file would work for unprivileged accounts, but root (in particular,
	// in the Docker container we use for testing) would still have access.  So, create a symlink
	// pointing to itself, to cause an ELOOP. (Note that a symlink pointing to a nonexistent file would be treated
	// just like a nonexistent signature file, and not an error.)
	err = os.Symlink("signature-1", path.Join(dir, "signature-1"))
	require.NoError(t, err)
	return dir
}

func TestPRSignedByIsRunningImageAllowed(t *testing.T) {
	ktGPG := SBKeyTypeGPGKeys
	prm := NewPRMMatchExact()

	// A simple success case: single valid signature.
	image := dirImageMock(t, "fixtures/dir-img-valid", "testing/manifest:latest")
	pr, err := NewPRSignedByKeyPath(ktGPG, "fixtures/public-key.gpg", prm)
	require.NoError(t, err)
	allowed, err := pr.isRunningImageAllowed(context.Background(), image)
	assertRunningAllowed(t, allowed, err)

	// Error reading signatures
	invalidSigDir := createInvalidSigDir(t)
	image = dirImageMock(t, invalidSigDir, "testing/manifest:latest")
	pr, err = NewPRSignedByKeyPath(ktGPG, "fixtures/public-key.gpg", prm)
	require.NoError(t, err)
	allowed, err = pr.isRunningImageAllowed(context.Background(), image)
	assertRunningRejected(t, allowed, err)

	// No signatures
	image = dirImageMock(t, "fixtures/dir-img-unsigned", "testing/manifest:latest")
	pr, err = NewPRSignedByKeyPath(ktGPG, "fixtures/public-key.gpg", prm)
	require.NoError(t, err)
	allowed, err = pr.isRunningImageAllowed(context.Background(), image)
	assertRunningRejectedPolicyRequirement(t, allowed, err)

	// 1 invalid signature: use dir-img-valid, but a non-matching Docker reference
	image = dirImageMock(t, "fixtures/dir-img-valid", "testing/manifest:notlatest")
	pr, err = NewPRSignedByKeyPath(ktGPG, "fixtures/public-key.gpg", prm)
	require.NoError(t, err)
	allowed, err = pr.isRunningImageAllowed(context.Background(), image)
	assertRunningRejectedPolicyRequirement(t, allowed, err)

	// 2 valid signatures
	image = dirImageMock(t, "fixtures/dir-img-valid-2", "testing/manifest:latest")
	pr, err = NewPRSignedByKeyPath(ktGPG, "fixtures/public-key.gpg", prm)
	require.NoError(t, err)
	allowed, err = pr.isRunningImageAllowed(context.Background(), image)
	assertRunningAllowed(t, allowed, err)

	// One invalid, one valid signature (in this order)
	image = dirImageMock(t, "fixtures/dir-img-mixed", "testing/manifest:latest")
	pr, err = NewPRSignedByKeyPath(ktGPG, "fixtures/public-key.gpg", prm)
	require.NoError(t, err)
	allowed, err = pr.isRunningImageAllowed(context.Background(), image)
	assertRunningAllowed(t, allowed, err)

	// 2 invalid signatures: use dir-img-valid-2, but a non-matching Docker reference
	image = dirImageMock(t, "fixtures/dir-img-valid-2", "testing/manifest:notlatest")
	pr, err = NewPRSignedByKeyPath(ktGPG, "fixtures/public-key.gpg", prm)
	require.NoError(t, err)
	allowed, err = pr.isRunningImageAllowed(context.Background(), image)
	assertRunningRejectedPolicyRequirement(t, allowed, err)
}

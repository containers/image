package layout

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/containers/image/v5/internal/private"
	"github.com/containers/image/v5/internal/signature"
	"github.com/containers/image/v5/pkg/blobinfocache/memory"
	"github.com/containers/image/v5/types"
	digest "github.com/opencontainers/go-digest"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ private.ImageDestination = (*ociImageDestination)(nil)

// readerFromFunc allows implementing Reader by any function, e.g. a closure.
type readerFromFunc func([]byte) (int, error)

func (fn readerFromFunc) Read(p []byte) (int, error) {
	return fn(p)
}

// TestPutBlobDigestFailure simulates behavior on digest verification failure.
func TestPutBlobDigestFailure(t *testing.T) {
	const digestErrorString = "Simulated digest error"
	const blobDigest = "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f"

	ref, _ := refToTempOCI(t, false)
	dirRef, ok := ref.(ociReference)
	require.True(t, ok)
	blobPath, err := dirRef.blobPath(blobDigest, "")
	assert.NoError(t, err)
	cache := memory.New()

	firstRead := true
	reader := readerFromFunc(func(p []byte) (int, error) {
		_, err := os.Lstat(blobPath)
		require.Error(t, err)
		require.True(t, os.IsNotExist(err))
		if firstRead {
			if len(p) > 0 {
				firstRead = false
			}
			for i := 0; i < len(p); i++ {
				p[i] = 0xAA
			}
			return len(p), nil
		}
		return 0, errors.New(digestErrorString)
	})

	dest, err := ref.NewImageDestination(context.Background(), nil)
	require.NoError(t, err)
	defer dest.Close()
	_, err = dest.PutBlob(context.Background(), reader, types.BlobInfo{Digest: blobDigest, Size: -1}, cache, false)
	assert.ErrorContains(t, err, digestErrorString)
	err = dest.Commit(context.Background(), nil) // nil unparsedToplevel is invalid, we don’t currently use the value
	assert.NoError(t, err)

	_, err = os.Lstat(blobPath)
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))
}

// TestPutManifestAppendsToExistingManifest tests that new manifests are getting added to existing index.
func TestPutManifestAppendsToExistingManifest(t *testing.T) {
	ref, tmpDir := refToTempOCI(t, false)

	ociRef, ok := ref.(ociReference)
	require.True(t, ok)

	// initially we have one manifest
	index, err := ociRef.getIndex()
	assert.NoError(t, err)
	assert.Equal(t, 1, len(index.Manifests), "Unexpected number of manifests")

	// create a new test reference
	ociRef2, err := NewReference(tmpDir, "new-image")
	assert.NoError(t, err)

	putTestManifest(t, ociRef2.(ociReference), tmpDir)

	index, err = ociRef.getIndex()
	assert.NoError(t, err)
	assert.Equal(t, 2, len(index.Manifests), "Unexpected number of manifests")
}

// TestPutManifestTwice tests that existing manifest gets updated and not appended.
func TestPutManifestTwice(t *testing.T) {
	ref, tmpDir := refToTempOCI(t, false)

	ociRef, ok := ref.(ociReference)
	require.True(t, ok)

	putTestConfig(t, ociRef, tmpDir)
	putTestManifest(t, ociRef, tmpDir)
	putTestManifest(t, ociRef, tmpDir)

	index, err := ociRef.getIndex()
	assert.NoError(t, err)
	assert.Len(t, index.Manifests, 2, "Unexpected number of manifests")
}

func TestPutTwoDifferentTags(t *testing.T) {
	ref, tmpDir := refToTempOCI(t, false)

	ociRef, ok := ref.(ociReference)
	require.True(t, ok)

	putTestConfig(t, ociRef, tmpDir)
	putTestManifest(t, ociRef, tmpDir)

	// add the same manifest with a different tag; it shouldn't get overwritten
	ref, err := NewReference(tmpDir, "zomg")
	assert.NoError(t, err)
	ociRef, ok = ref.(ociReference)
	require.True(t, ok)
	putTestManifest(t, ociRef, tmpDir)

	index, err := ociRef.getIndex()
	assert.NoError(t, err)
	assert.Len(t, index.Manifests, 3, "Unexpected number of manifests")
	assert.Equal(t, "imageValue", index.Manifests[1].Annotations[imgspecv1.AnnotationRefName])
	assert.Equal(t, "zomg", index.Manifests[2].Annotations[imgspecv1.AnnotationRefName])
}

func putTestConfig(t *testing.T, ociRef ociReference, tmpDir string) {
	data, err := os.ReadFile("../../internal/image/fixtures/oci1-config.json")
	assert.NoError(t, err)
	imageDest, err := newImageDestination(nil, ociRef)
	assert.NoError(t, err)

	cache := memory.New()

	_, err = imageDest.PutBlob(context.Background(), bytes.NewReader(data), types.BlobInfo{Size: int64(len(data)), Digest: digest.FromBytes(data)}, cache, true)
	assert.NoError(t, err)

	err = imageDest.Commit(context.Background(), nil) // nil unparsedToplevel is invalid, we don’t currently use the value
	assert.NoError(t, err)

	paths := []string{}
	err = filepath.WalkDir(tmpDir, func(path string, _ fs.DirEntry, err error) error {
		paths = append(paths, path)
		return nil
	})
	assert.NoError(t, err)

	digest := digest.FromBytes(data).Encoded()
	assert.Contains(t, paths, filepath.Join(tmpDir, "blobs", "sha256", digest), "The OCI directory does not contain the new config data")
}

func putTestManifest(t *testing.T, ociRef ociReference, tmpDir string) {
	data, err := os.ReadFile("../../internal/image/fixtures/oci1.json")
	assert.NoError(t, err)
	imageDest, err := newImageDestination(nil, ociRef)
	assert.NoError(t, err)

	err = imageDest.PutManifest(context.Background(), data, nil)
	assert.NoError(t, err)

	err = imageDest.Commit(context.Background(), nil) // nil unparsedToplevel is invalid, we don’t currently use the value
	assert.NoError(t, err)

	paths := []string{}
	err = filepath.WalkDir(tmpDir, func(path string, _ fs.DirEntry, err error) error {
		paths = append(paths, path)
		return nil
	})
	assert.NoError(t, err)

	digest := digest.FromBytes(data).Encoded()
	assert.Contains(t, paths, filepath.Join(tmpDir, "blobs", "sha256", digest), "The OCI directory does not contain the new manifest data")
}

func TestPutblobFromLocalFile(t *testing.T) {
	ref, _ := refToTempOCI(t, false)
	dest, err := ref.NewImageDestination(context.Background(), nil)
	require.NoError(t, err)
	defer dest.Close()
	ociDest, ok := dest.(*ociImageDestination)
	require.True(t, ok)

	for _, test := range []struct {
		path   string
		size   int64
		digest string
	}{
		{path: "fixtures/files/a.txt", size: 31, digest: "sha256:c8a3f498ce6aaa13c803fa3a6a0d5fd6b5d75be5781f98f56c0f960efcc53174"},
		{path: "fixtures/files/b.txt", size: 25, digest: "sha256:8c1e9b03116b95e6dfac68c588190d56bfc82b9cc550ada726e882e138a3b93b"},
		{path: "fixtures/files/b.txt", size: 25, digest: "sha256:8c1e9b03116b95e6dfac68c588190d56bfc82b9cc550ada726e882e138a3b93b"}, // Must not fail
	} {
		digest, size, err := PutBlobFromLocalFile(context.Background(), dest, test.path)
		require.NoError(t, err)
		require.Equal(t, test.size, size)
		require.Equal(t, test.digest, digest.String())

		blobPath, err := ociDest.ref.blobPath(digest, ociDest.sharedBlobDir)
		require.NoError(t, err)
		require.FileExists(t, blobPath)

		expectedContent, err := os.ReadFile(test.path)
		require.NoError(t, err)
		require.NotEmpty(t, expectedContent)
		blobContent, err := os.ReadFile(blobPath)
		require.NoError(t, err)
		require.Equal(t, expectedContent, blobContent)
	}

	err = ociDest.CommitWithOptions(context.Background(), private.CommitOptions{})
	require.NoError(t, err)
}

// TestPutSignaturesWithFormat tests that sigstore signatures are properly stored in OCI layout
func TestPutSignaturesWithFormat(t *testing.T) {
	ref, tmpDir := refToTempOCI(t, false)
	ociRef, ok := ref.(ociReference)
	require.True(t, ok)
	putTestManifest(t, ociRef, tmpDir)

	dest, err := ref.NewImageDestination(context.Background(), nil)
	require.NoError(t, err)
	defer dest.Close()
	ociDest, ok := dest.(*ociImageDestination)
	require.True(t, ok)

	desc, _, err := ociDest.ref.getManifestDescriptor()
	require.NoError(t, err)
	require.NotNil(t, desc)

	sigstoreSign := signature.SigstoreFromComponents(
		"application/vnd.dev.cosign.simplesigning.v1+json",
		[]byte("test-payload"),
		map[string]string{"dev.cosignproject.cosign/signature": "test-signature"},
	)

	err = ociDest.PutSignaturesWithFormat(context.Background(), []signature.Signature{sigstoreSign}, &desc.Digest)
	require.NoError(t, err)

	err = ociDest.Commit(context.Background(), nil)
	require.NoError(t, err)

	src, err := ref.NewImageSource(context.Background(), nil)
	require.NoError(t, err)
	ociSrc, ok := src.(*ociImageSource)
	require.True(t, ok)
	sign, err := ociSrc.GetSignaturesWithFormat(context.Background(), &desc.Digest)
	require.NoError(t, err)
	require.Len(t, sign, 1)
	require.Equal(t, sigstoreSign, sign[0])
}

// TestPutSignaturesWithFormatNilDigest tests error handling when instanceDigest is nil
func TestPutSignaturesWithFormatNilDigest(t *testing.T) {
	ref, _ := refToTempOCI(t, false)

	dest, err := ref.NewImageDestination(context.Background(), nil)
	require.NoError(t, err)
	defer dest.Close()

	// Cast to ociImageDestination to access PutSignaturesWithFormat
	ociDest, ok := dest.(*ociImageDestination)
	require.True(t, ok)

	// Create a test signature
	testPayload := []byte(`{"test": "payload"}`)
	testAnnotations := map[string]string{
		"dev.cosignproject.cosign/signature": "test-signature",
	}
	sig := signature.SigstoreFromComponents("application/vnd.dev.cosign.simplesigning.v1+json", testPayload, testAnnotations)

	// Test that PutSignaturesWithFormat fails when instanceDigest is nil
	err = ociDest.PutSignaturesWithFormat(context.Background(), []signature.Signature{sig}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown manifest digest, can't add signatures")
}

// TestPutSignaturesWithFormatNonSigstore tests error handling for non-sigstore signatures
func TestPutSignaturesWithFormatNonSigstore(t *testing.T) {
	ref, _ := refToTempOCI(t, false)

	dest, err := ref.NewImageDestination(context.Background(), nil)
	require.NoError(t, err)
	defer dest.Close()

	// Cast to ociImageDestination to access PutSignaturesWithFormat
	ociDest, ok := dest.(*ociImageDestination)
	require.True(t, ok)

	// Create a non-sigstore signature (simple signing)
	simpleSig := signature.SimpleSigningFromBlob([]byte("simple signature data"))
	testDigest := digest.FromString("test-manifest")

	// Test that PutSignaturesWithFormat fails for non-sigstore signatures
	err = ociDest.PutSignaturesWithFormat(context.Background(), []signature.Signature{simpleSig}, &testDigest)
	require.Error(t, err)
	require.Contains(t, err.Error(), "OCI Layout only supports sigstoreSignatures")
}

package directory

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/containers/image/v5/internal/private"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/pkg/blobinfocache/memory"
	"github.com/containers/image/v5/types"
	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ private.ImageSource = (*dirImageSource)(nil)
var _ private.ImageDestination = (*dirImageDestination)(nil)

func TestDestinationReference(t *testing.T) {
	ref, tmpDir := refToTempDir(t)

	dest, err := ref.NewImageDestination(context.Background(), nil)
	require.NoError(t, err)
	defer dest.Close()
	ref2 := dest.Reference()
	assert.Equal(t, tmpDir, ref2.StringWithinTransport())
}

func TestGetPutManifest(t *testing.T) {
	ref, _ := refToTempDir(t)

	man := []byte("test-manifest")
	list := []byte("test-manifest-list")
	md, err := manifest.Digest(man)
	require.NoError(t, err)
	dest, err := ref.NewImageDestination(context.Background(), nil)
	require.NoError(t, err)
	defer dest.Close()
	err = dest.PutManifest(context.Background(), man, &md)
	assert.NoError(t, err)
	err = dest.PutManifest(context.Background(), list, nil)
	assert.NoError(t, err)
	err = dest.Commit(context.Background(), nil) // nil unparsedToplevel is invalid, we don’t currently use the value
	assert.NoError(t, err)

	src, err := ref.NewImageSource(context.Background(), nil)
	require.NoError(t, err)
	defer src.Close()
	m, mt, err := src.GetManifest(context.Background(), nil)
	assert.NoError(t, err)
	assert.Equal(t, list, m)
	assert.Equal(t, "", mt)

	m, mt, err = src.GetManifest(context.Background(), &md)
	assert.NoError(t, err)
	assert.Equal(t, man, m)
	assert.Equal(t, "", mt)
}

func TestGetPutBlob(t *testing.T) {
	computedBlob := []byte("test-blob")
	providedBlob := []byte("provided-blob")
	providedDigest := digest.Digest("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	ref, _ := refToTempDir(t)
	cache := memory.New()

	dest, err := ref.NewImageDestination(context.Background(), nil)
	require.NoError(t, err)
	defer dest.Close()
	assert.Equal(t, types.PreserveOriginal, dest.DesiredLayerCompression())
	// PutBlob with caller-provided data
	providedInfo, err := dest.PutBlob(context.Background(), bytes.NewReader(providedBlob), types.BlobInfo{Digest: providedDigest, Size: int64(len(providedBlob))}, cache, false)
	assert.NoError(t, err)
	assert.Equal(t, int64(len(providedBlob)), providedInfo.Size)
	assert.Equal(t, providedDigest, providedInfo.Digest)
	// PutBlob with unknown data
	computedInfo, err := dest.PutBlob(context.Background(), bytes.NewReader(computedBlob), types.BlobInfo{Digest: "", Size: int64(-1)}, cache, false)
	assert.NoError(t, err)
	assert.Equal(t, int64(len(computedBlob)), computedInfo.Size)
	assert.Equal(t, digest.FromBytes(computedBlob), computedInfo.Digest)
	err = dest.Commit(context.Background(), nil) // nil unparsedToplevel is invalid, we don’t currently use the value
	assert.NoError(t, err)

	src, err := ref.NewImageSource(context.Background(), nil)
	require.NoError(t, err)
	defer src.Close()
	for digest, expectedBlob := range map[digest.Digest][]byte{
		providedInfo.Digest: providedBlob,
		computedInfo.Digest: computedBlob,
	} {
		rc, size, err := src.GetBlob(context.Background(), types.BlobInfo{Digest: digest, Size: int64(len(expectedBlob))}, cache)
		assert.NoError(t, err)
		defer rc.Close()
		b, err := io.ReadAll(rc)
		assert.NoError(t, err)
		assert.Equal(t, expectedBlob, b)
		assert.Equal(t, int64(len(expectedBlob)), size)
	}
}

// readerFromFunc allows implementing Reader by any function, e.g. a closure.
type readerFromFunc func([]byte) (int, error)

func (fn readerFromFunc) Read(p []byte) (int, error) {
	return fn(p)
}

// TestPutBlobDigestFailure simulates behavior on digest verification failure.
func TestPutBlobDigestFailure(t *testing.T) {
	const digestErrorString = "Simulated digest error"
	const blobDigest = digest.Digest("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	ref, _ := refToTempDir(t)
	dirRef, ok := ref.(dirReference)
	require.True(t, ok)
	blobPath, err := dirRef.layerPath(blobDigest)
	require.NoError(t, err)
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

func TestGetPutSignatures(t *testing.T) {
	ref, _ := refToTempDir(t)

	man := []byte("test-manifest")
	list := []byte("test-manifest-list")
	md, err := manifest.Digest(man)
	require.NoError(t, err)
	// These signatures are completely invalid; start with 0xA3 just to be minimally plausible to signature.FromBlob.
	signatures := [][]byte{
		[]byte("\xA3sig1"),
		[]byte("\xA3sig2"),
	}
	listSignatures := [][]byte{
		[]byte("\xA3sig3"),
		[]byte("\xA3sig4"),
	}

	dest, err := ref.NewImageDestination(context.Background(), nil)
	require.NoError(t, err)
	defer dest.Close()
	err = dest.SupportsSignatures(context.Background())
	assert.NoError(t, err)

	err = dest.PutManifest(context.Background(), man, &md)
	require.NoError(t, err)
	err = dest.PutManifest(context.Background(), list, nil)
	require.NoError(t, err)

	err = dest.PutSignatures(context.Background(), signatures, &md)
	assert.NoError(t, err)
	err = dest.PutSignatures(context.Background(), listSignatures, nil)
	assert.NoError(t, err)
	err = dest.Commit(context.Background(), nil) // nil unparsedToplevel is invalid, we don’t currently use the value
	assert.NoError(t, err)

	src, err := ref.NewImageSource(context.Background(), nil)
	require.NoError(t, err)
	defer src.Close()
	sigs, err := src.GetSignatures(context.Background(), nil)
	assert.NoError(t, err)
	assert.Equal(t, listSignatures, sigs)

	sigs, err = src.GetSignatures(context.Background(), &md)
	assert.NoError(t, err)
	assert.Equal(t, signatures, sigs)
}

func TestSourceReference(t *testing.T) {
	ref, tmpDir := refToTempDir(t)

	src, err := ref.NewImageSource(context.Background(), nil)
	require.NoError(t, err)
	defer src.Close()
	ref2 := src.Reference()
	assert.Equal(t, tmpDir, ref2.StringWithinTransport())
}

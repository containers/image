package directory

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDestinationReference(t *testing.T) {
	ref, tmpDir := refToTempDir(t)
	defer os.RemoveAll(tmpDir)

	dest, err := ref.NewImageDestination(nil)
	require.NoError(t, err)
	ref2 := dest.Reference()
	assert.Equal(t, tmpDir, ref2.StringWithinTransport())
}

func TestGetPutManifest(t *testing.T) {
	ref, tmpDir := refToTempDir(t)
	defer os.RemoveAll(tmpDir)

	man := []byte("test-manifest")
	dest, err := ref.NewImageDestination(nil)
	require.NoError(t, err)
	err = dest.PutManifest(man)
	assert.NoError(t, err)

	src, err := ref.NewImageSource(nil, nil)
	require.NoError(t, err)
	m, mt, err := src.GetManifest()
	assert.NoError(t, err)
	assert.Equal(t, man, m)
	assert.Equal(t, "", mt)
}

func TestGetPutBlob(t *testing.T) {
	ref, tmpDir := refToTempDir(t)
	defer os.RemoveAll(tmpDir)

	digest := "digest-test"
	blob := []byte("test-blob")
	dest, err := ref.NewImageDestination(nil)
	require.NoError(t, err)
	err = dest.PutBlob(digest, bytes.NewReader(blob))
	assert.NoError(t, err)

	src, err := ref.NewImageSource(nil, nil)
	require.NoError(t, err)
	rc, size, err := src.GetBlob(digest)
	assert.NoError(t, err)
	defer rc.Close()
	b, err := ioutil.ReadAll(rc)
	assert.NoError(t, err)
	assert.Equal(t, blob, b)
	assert.Equal(t, int64(len(blob)), size)
}

// readerFromFunc allows implementing Reader by any function, e.g. a closure.
type readerFromFunc func([]byte) (int, error)

func (fn readerFromFunc) Read(p []byte) (int, error) {
	return fn(p)
}

// TestPutBlobDigestFailure simulates behavior on digest verification failure.
func TestPutBlobDigestFailure(t *testing.T) {
	const digestErrorString = "Simulated digest error"
	const blobDigest = "test-digest"

	ref, tmpDir := refToTempDir(t)
	defer os.RemoveAll(tmpDir)
	dirRef, ok := ref.(dirReference)
	require.True(t, ok)
	blobPath := dirRef.layerPath(blobDigest)

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
		return 0, fmt.Errorf(digestErrorString)
	})

	dest, err := ref.NewImageDestination(nil)
	require.NoError(t, err)
	err = dest.PutBlob(blobDigest, reader)
	assert.Error(t, err)
	assert.Contains(t, digestErrorString, err.Error())

	_, err = os.Lstat(blobPath)
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))
}

func TestGetPutSignatures(t *testing.T) {
	ref, tmpDir := refToTempDir(t)
	defer os.RemoveAll(tmpDir)

	dest, err := ref.NewImageDestination(nil)
	require.NoError(t, err)
	signatures := [][]byte{
		[]byte("sig1"),
		[]byte("sig2"),
	}
	err = dest.PutSignatures(signatures)
	assert.NoError(t, err)

	src, err := ref.NewImageSource(nil, nil)
	require.NoError(t, err)
	sigs, err := src.GetSignatures()
	assert.NoError(t, err)
	assert.Equal(t, signatures, sigs)
}

func TestSourceReference(t *testing.T) {
	ref, tmpDir := refToTempDir(t)
	defer os.RemoveAll(tmpDir)

	src, err := ref.NewImageSource(nil, nil)
	require.NoError(t, err)
	ref2 := src.Reference()
	assert.Equal(t, tmpDir, ref2.StringWithinTransport())
}

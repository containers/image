package directory

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/containers/image/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDestinationReference(t *testing.T) {
	ref, tmpDir := refToTempDir(t)
	defer os.RemoveAll(tmpDir)

	dest, err := ref.NewImageDestination(nil)
	require.NoError(t, err)
	defer dest.Close()
	ref2 := dest.Reference()
	assert.Equal(t, tmpDir, ref2.StringWithinTransport())
}

func TestGetPutManifest(t *testing.T) {
	ref, tmpDir := refToTempDir(t)
	defer os.RemoveAll(tmpDir)

	man := []byte("test-manifest")
	dest, err := ref.NewImageDestination(nil)
	require.NoError(t, err)
	defer dest.Close()
	err = dest.PutManifest(man)
	assert.NoError(t, err)
	err = dest.Commit()
	assert.NoError(t, err)

	src, err := ref.NewImageSource(nil, nil)
	require.NoError(t, err)
	defer src.Close()
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
	defer dest.Close()
	info, err := dest.PutBlob(bytes.NewReader(blob), types.BlobInfo{Digest: digest, Size: int64(9)})
	assert.NoError(t, err)
	err = dest.Commit()
	assert.NoError(t, err)
	assert.Equal(t, int64(9), info.Size)
	hash := sha256.Sum256(blob)
	assert.Equal(t, "sha256:"+hex.EncodeToString(hash[:]), info.Digest)

	src, err := ref.NewImageSource(nil, nil)
	require.NoError(t, err)
	defer src.Close()
	rc, size, err := src.GetBlob(info.Digest)
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
	defer dest.Close()
	_, err = dest.PutBlob(reader, types.BlobInfo{Digest: blobDigest, Size: -1})
	assert.Error(t, err)
	assert.Contains(t, digestErrorString, err.Error())
	err = dest.Commit()
	assert.NoError(t, err)

	_, err = os.Lstat(blobPath)
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))
}

func TestGetPutSignatures(t *testing.T) {
	ref, tmpDir := refToTempDir(t)
	defer os.RemoveAll(tmpDir)

	dest, err := ref.NewImageDestination(nil)
	require.NoError(t, err)
	defer dest.Close()
	signatures := [][]byte{
		[]byte("sig1"),
		[]byte("sig2"),
	}
	err = dest.SupportsSignatures()
	assert.NoError(t, err)
	err = dest.PutSignatures(signatures)
	assert.NoError(t, err)
	err = dest.Commit()
	assert.NoError(t, err)

	src, err := ref.NewImageSource(nil, nil)
	require.NoError(t, err)
	defer src.Close()
	sigs, err := src.GetSignatures()
	assert.NoError(t, err)
	assert.Equal(t, signatures, sigs)
}

func TestSourceReference(t *testing.T) {
	ref, tmpDir := refToTempDir(t)
	defer os.RemoveAll(tmpDir)

	src, err := ref.NewImageSource(nil, nil)
	require.NoError(t, err)
	defer src.Close()
	ref2 := src.Reference()
	assert.Equal(t, tmpDir, ref2.StringWithinTransport())
}

package directory

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDestinationReference(t *testing.T) {
	ref, tmpDir := refToTempDir(t)
	defer os.RemoveAll(tmpDir)

	dest, err := ref.NewImageDestination("", true)
	require.NoError(t, err)
	ref2 := dest.Reference()
	assert.Equal(t, tmpDir, ref2.StringWithinTransport())
}

func TestGetPutManifest(t *testing.T) {
	ref, tmpDir := refToTempDir(t)
	defer os.RemoveAll(tmpDir)

	man := []byte("test-manifest")
	dest, err := ref.NewImageDestination("", true)
	require.NoError(t, err)
	err = dest.PutManifest(man)
	assert.NoError(t, err)

	src, err := ref.NewImageSource("", true)
	require.NoError(t, err)
	m, mt, err := src.GetManifest(nil)
	assert.NoError(t, err)
	assert.Equal(t, man, m)
	assert.Equal(t, "", mt)
}

func TestGetPutBlob(t *testing.T) {
	ref, tmpDir := refToTempDir(t)
	defer os.RemoveAll(tmpDir)

	digest := "digest-test"
	blob := []byte("test-blob")
	dest, err := ref.NewImageDestination("", true)
	require.NoError(t, err)
	err = dest.PutBlob(digest, bytes.NewReader(blob))
	assert.NoError(t, err)

	src, err := ref.NewImageSource("", true)
	require.NoError(t, err)
	rc, size, err := src.GetBlob(digest)
	assert.NoError(t, err)
	defer rc.Close()
	b, err := ioutil.ReadAll(rc)
	assert.NoError(t, err)
	assert.Equal(t, blob, b)
	assert.Equal(t, int64(len(blob)), size)
}

func TestGetPutSignatures(t *testing.T) {
	ref, tmpDir := refToTempDir(t)
	defer os.RemoveAll(tmpDir)

	dest, err := ref.NewImageDestination("", true)
	require.NoError(t, err)
	signatures := [][]byte{
		[]byte("sig1"),
		[]byte("sig2"),
	}
	err = dest.PutSignatures(signatures)
	assert.NoError(t, err)

	src, err := ref.NewImageSource("", true)
	require.NoError(t, err)
	sigs, err := src.GetSignatures()
	assert.NoError(t, err)
	assert.Equal(t, signatures, sigs)
}

func TestDelete(t *testing.T) {
	ref, tmpDir := refToTempDir(t)
	defer os.RemoveAll(tmpDir)

	src, err := ref.NewImageSource("", true)
	require.NoError(t, err)
	err = src.Delete()
	assert.Error(t, err)
}

func TestSourceReference(t *testing.T) {
	ref, tmpDir := refToTempDir(t)
	defer os.RemoveAll(tmpDir)

	src, err := ref.NewImageSource("", true)
	require.NoError(t, err)
	ref2 := src.Reference()
	assert.Equal(t, tmpDir, ref2.StringWithinTransport())
}

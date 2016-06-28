package directory

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanonicalDockerReference(t *testing.T) {
	dest := NewDirImageDestination("/path/to/somewhere")
	_, err := dest.CanonicalDockerReference()
	assert.Error(t, err)
}

func TestGetPutManifest(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "put-manifest")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	man := []byte("test-manifest")
	dest := NewDirImageDestination(tmpDir)
	err = dest.PutManifest(man)
	assert.NoError(t, err)

	src := NewDirImageSource(tmpDir)
	m, mt, err := src.GetManifest(nil)
	assert.NoError(t, err)
	assert.Equal(t, man, m)
	assert.Equal(t, "", mt)
}

func TestGetPutBlob(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "put-blob")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	digest := "digest-test"
	blob := []byte("test-blob")
	dest := NewDirImageDestination(tmpDir)
	err = dest.PutBlob(digest, bytes.NewReader(blob))
	assert.NoError(t, err)

	src := NewDirImageSource(tmpDir)
	rc, size, err := src.GetBlob(digest)
	assert.NoError(t, err)
	defer rc.Close()
	b, err := ioutil.ReadAll(rc)
	assert.NoError(t, err)
	assert.Equal(t, blob, b)
	assert.Equal(t, int64(len(blob)), size)
}

func TestGetPutSignatures(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "put-signatures")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	dest := NewDirImageDestination(tmpDir)
	signatures := [][]byte{
		[]byte("sig1"),
		[]byte("sig2"),
	}
	err = dest.PutSignatures(signatures)
	assert.NoError(t, err)

	src := NewDirImageSource(tmpDir)
	sigs, err := src.GetSignatures()
	assert.NoError(t, err)
	assert.Equal(t, signatures, sigs)
}

func TestDelete(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "delete")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	src := NewDirImageSource(tmpDir)
	err = src.Delete()
	assert.Error(t, err)
}

func TestIntendedDockerReference(t *testing.T) {
	src := NewDirImageSource("/path/to/somewhere")
	dr := src.IntendedDockerReference()
	assert.Equal(t, "", dr)

}

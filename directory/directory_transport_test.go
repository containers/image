package directory

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/containers/image/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransportName(t *testing.T) {
	assert.Equal(t, "dir", Transport.Name())
}

func TestTransportParseReference(t *testing.T) {
	testNewReference(t, Transport.ParseReference)
}

func TestNewReference(t *testing.T) {
	testNewReference(t, func(ref string) (types.ImageReference, error) {
		return NewReference(ref), nil
	})
}

// testNewReference is a test shared for Transport.ParseReference and NewReference.
func testNewReference(t *testing.T, fn func(string) (types.ImageReference, error)) {
	tmpDir, err := ioutil.TempDir("", "dir-transport-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	for _, path := range []string{
		"/",
		"/etc",
		tmpDir,
		"relativepath",
		tmpDir + "/thisdoesnotexist",
	} {
		ref, err := fn(path)
		require.NoError(t, err, path)
		dirRef, ok := ref.(dirReference)
		require.True(t, ok)
		assert.Equal(t, path, dirRef.path, path)
	}
}

// refToTempDir creates a temporary directory and returns a reference to it.
// The caller should
//   defer os.RemoveAll(tmpDir)
func refToTempDir(t *testing.T) (ref types.ImageReference, tmpDir string) {
	tmpDir, err := ioutil.TempDir("", "dir-transport-test")
	require.NoError(t, err)
	ref = NewReference(tmpDir)
	return ref, tmpDir
}

func TestReferenceTransport(t *testing.T) {
	ref, tmpDir := refToTempDir(t)
	defer os.RemoveAll(tmpDir)
	assert.Equal(t, Transport, ref.Transport())
}

func TestReferenceStringWithinTransport(t *testing.T) {
	ref, tmpDir := refToTempDir(t)
	defer os.RemoveAll(tmpDir)
	assert.Equal(t, tmpDir, ref.StringWithinTransport())
}

func TestReferenceNewImage(t *testing.T) {
	ref, tmpDir := refToTempDir(t)
	defer os.RemoveAll(tmpDir)
	_, err := ref.NewImage("/this/doesn't/exist", true)
	assert.NoError(t, err)
}

func TestReferenceNewImageSource(t *testing.T) {
	ref, tmpDir := refToTempDir(t)
	defer os.RemoveAll(tmpDir)
	_, err := ref.NewImageSource("/this/doesn't/exist", true)
	assert.NoError(t, err)
}

func TestReferenceNewImageDestination(t *testing.T) {
	ref, tmpDir := refToTempDir(t)
	defer os.RemoveAll(tmpDir)
	_, err := ref.NewImageDestination("/this/doesn't/exist", true)
	assert.NoError(t, err)
}

package oci

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/containers/image/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransportName(t *testing.T) {
	assert.Equal(t, "oci", Transport.Name())
}

func TestTransportParseReference(t *testing.T) {
	testParseReference(t, Transport.ParseReference)
}

func TestParseReference(t *testing.T) {
	testParseReference(t, ParseReference)
}

// testParseReference is a test shared for Transport.ParseReference and ParseReference.
func testParseReference(t *testing.T, fn func(string) (types.ImageReference, error)) {
	tmpDir, err := ioutil.TempDir("", "oci-transport-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	for _, path := range []string{
		"/",
		"/etc",
		tmpDir,
		"relativepath",
		tmpDir + "/thisdoesnotexist",
	} {
		for _, tag := range []struct{ suffix, tag string }{
			{":notlatest", "notlatest"},
			{"", "latest"},
		} {
			input := path + tag.suffix
			ref, err := fn(input)
			require.NoError(t, err, input)
			ociRef, ok := ref.(ociReference)
			require.True(t, ok)
			assert.Equal(t, path, ociRef.dir, input)
			assert.Equal(t, tag.tag, ociRef.tag, input)
		}
	}

	ref, err := fn(tmpDir + "/with:colons:and:tag")
	require.NoError(t, err)
	ociRef, ok := ref.(ociReference)
	require.True(t, ok)
	assert.Equal(t, tmpDir+"/with:colons:and", ociRef.dir)
	assert.Equal(t, "tag", ociRef.tag)

	_, err = fn(tmpDir + ":invalid'tag!value@")
	assert.Error(t, err)
}

func TestNewReference(t *testing.T) {
	const tagValue = "tagValue"

	tmpDir, err := ioutil.TempDir("", "oci-transport-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	ref := NewReference(tmpDir, tagValue)
	ociRef, ok := ref.(ociReference)
	require.True(t, ok)
	assert.Equal(t, tmpDir, ociRef.dir)
	assert.Equal(t, tagValue, ociRef.tag)
}

// refToTempOCI creates a temporary directory and returns an reference to it.
// The caller should
//   defer os.RemoveAll(tmpDir)
func refToTempOCI(t *testing.T) (ref types.ImageReference, tmpDir string) {
	tmpDir, err := ioutil.TempDir("", "oci-transport-test")
	require.NoError(t, err)
	ref = NewReference(tmpDir, "tagValue")
	return ref, tmpDir
}

func TestReferenceTransport(t *testing.T) {
	ref, tmpDir := refToTempOCI(t)
	defer os.RemoveAll(tmpDir)
	assert.Equal(t, Transport, ref.Transport())
}

func TestReferenceStringWithinTransport(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "oci-transport-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	for _, c := range []struct{ input, result string }{
		{"/dir1:notlatest", "/dir1:notlatest"}, // Explicit tag
		{"/dir2", "/dir2:latest"},              // Default tag
		{"/dir3:with:colons:and:tag", "/dir3:with:colons:and:tag"},
	} {
		ref, err := ParseReference(tmpDir + c.input)
		require.NoError(t, err, c.input)
		stringRef := ref.StringWithinTransport()
		assert.Equal(t, tmpDir+c.result, stringRef, c.input)
		// Do one more round to verify that the output can be parsed, to an equal value.
		ref2, err := Transport.ParseReference(stringRef)
		require.NoError(t, err, c.input)
		stringRef2 := ref2.StringWithinTransport()
		assert.Equal(t, stringRef, stringRef2, c.input)
	}
}

func TestReferenceDockerReference(t *testing.T) {
	ref, tmpDir := refToTempOCI(t)
	defer os.RemoveAll(tmpDir)
	assert.Nil(t, ref.DockerReference())
}

func TestReferencePolicyConfigurationIdentity(t *testing.T) {
	ref, tmpDir := refToTempOCI(t)
	defer os.RemoveAll(tmpDir)
	assert.Equal(t, "", ref.PolicyConfigurationIdentity())
}

func TestReferencePolicyConfigurationNamespaces(t *testing.T) {
	ref, tmpDir := refToTempOCI(t)
	defer os.RemoveAll(tmpDir)
	assert.Nil(t, ref.PolicyConfigurationNamespaces())
}

func TestReferenceNewImage(t *testing.T) {
	ref, tmpDir := refToTempOCI(t)
	defer os.RemoveAll(tmpDir)
	_, err := ref.NewImage("/this/doesn't/exist", true)
	assert.Error(t, err)
}

func TestReferenceNewImageSource(t *testing.T) {
	ref, tmpDir := refToTempOCI(t)
	defer os.RemoveAll(tmpDir)
	_, err := ref.NewImageSource("/this/doesn't/exist", true)
	assert.Error(t, err)
}

func TestReferenceNewImageDestination(t *testing.T) {
	ref, tmpDir := refToTempOCI(t)
	defer os.RemoveAll(tmpDir)
	_, err := ref.NewImageDestination("/this/doesn't/exist", true)
	assert.NoError(t, err)
}

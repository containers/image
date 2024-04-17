package directory

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/containers/image/v5/internal/testing/explicitfilepath-tmpdir"
	"github.com/containers/image/v5/types"
	digest "github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransportName(t *testing.T) {
	assert.Equal(t, "dir", Transport.Name())
}

func TestTransportParseReference(t *testing.T) {
	testNewReference(t, Transport.ParseReference)
}

func TestTransportValidatePolicyConfigurationScope(t *testing.T) {
	for _, scope := range []string{
		"/etc",
		"/this/does/not/exist",
	} {
		err := Transport.ValidatePolicyConfigurationScope(scope)
		assert.NoError(t, err, scope)
	}

	for _, scope := range []string{
		"relative/path",
		"/double//slashes",
		"/has/./dot",
		"/has/dot/../dot",
		"/trailing/slash/",
		"/",
	} {
		err := Transport.ValidatePolicyConfigurationScope(scope)
		assert.Error(t, err, scope)
	}
}

func TestNewReference(t *testing.T) {
	testNewReference(t, NewReference)
}

// testNewReference is a test shared for Transport.ParseReference and NewReference.
func testNewReference(t *testing.T, fn func(string) (types.ImageReference, error)) {
	tmpDir := t.TempDir()

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

	_, err := fn(tmpDir + "/thisparentdoesnotexist/something")
	assert.Error(t, err)
}

// refToTempDir creates a temporary directory and returns a reference to it.
func refToTempDir(t *testing.T) (types.ImageReference, string) {
	tmpDir := t.TempDir()
	ref, err := NewReference(tmpDir)
	require.NoError(t, err)
	return ref, tmpDir
}

func TestReferenceTransport(t *testing.T) {
	ref, _ := refToTempDir(t)
	assert.Equal(t, Transport, ref.Transport())
}

func TestReferenceStringWithinTransport(t *testing.T) {
	ref, tmpDir := refToTempDir(t)
	assert.Equal(t, tmpDir, ref.StringWithinTransport())
}

func TestReferenceDockerReference(t *testing.T) {
	ref, _ := refToTempDir(t)
	assert.Nil(t, ref.DockerReference())
}

func TestReferencePolicyConfigurationIdentity(t *testing.T) {
	ref, tmpDir := refToTempDir(t)

	assert.Equal(t, tmpDir, ref.PolicyConfigurationIdentity())
	// A non-canonical path.  Test just one, the various other cases are
	// tested in explicitfilepath.ResolvePathToFullyExplicit.
	ref, err := NewReference(tmpDir + "/.")
	require.NoError(t, err)
	assert.Equal(t, tmpDir, ref.PolicyConfigurationIdentity())

	// "/" as a corner case.
	ref, err = NewReference("/")
	require.NoError(t, err)
	assert.Equal(t, "/", ref.PolicyConfigurationIdentity())
}

func TestReferencePolicyConfigurationNamespaces(t *testing.T) {
	ref, tmpDir := refToTempDir(t)
	// We don't really know enough to make a full equality test here.
	ns := ref.PolicyConfigurationNamespaces()
	require.NotNil(t, ns)
	assert.NotEmpty(t, ns)
	assert.Equal(t, filepath.Dir(tmpDir), ns[0])

	// Test with a known path which should exist. Test just one non-canonical
	// path, the various other cases are tested in explicitfilepath.ResolvePathToFullyExplicit.
	//
	// It would be nice to test a deeper hierarchy, but it is not obvious what
	// deeper path is always available in the various distros, AND is not likely
	// to contains a symbolic link.
	for _, path := range []string{"/usr/share", "/usr/share/./."} {
		_, err := os.Lstat(path)
		require.NoError(t, err)
		ref, err := NewReference(path)
		require.NoError(t, err)
		ns := ref.PolicyConfigurationNamespaces()
		require.NotNil(t, ns)
		assert.Equal(t, []string{"/usr"}, ns)
	}

	// "/" as a corner case.
	ref, err := NewReference("/")
	require.NoError(t, err)
	assert.Equal(t, []string{}, ref.PolicyConfigurationNamespaces())
}

func TestReferenceNewImage(t *testing.T) {
	ref, _ := refToTempDir(t)

	dest, err := ref.NewImageDestination(context.Background(), nil)
	require.NoError(t, err)
	defer dest.Close()
	mFixture, err := os.ReadFile("../manifest/fixtures/v2s1.manifest.json")
	require.NoError(t, err)
	err = dest.PutManifest(context.Background(), mFixture, nil)
	assert.NoError(t, err)
	err = dest.Commit(context.Background(), nil) // nil unparsedToplevel is invalid, we don’t currently use the value
	assert.NoError(t, err)

	img, err := ref.NewImage(context.Background(), nil)
	assert.NoError(t, err)
	defer img.Close()
}

func TestReferenceNewImageNoValidManifest(t *testing.T) {
	ref, _ := refToTempDir(t)

	dest, err := ref.NewImageDestination(context.Background(), nil)
	require.NoError(t, err)
	defer dest.Close()
	err = dest.PutManifest(context.Background(), []byte(`{"schemaVersion":1}`), nil)
	assert.NoError(t, err)
	err = dest.Commit(context.Background(), nil) // nil unparsedToplevel is invalid, we don’t currently use the value
	assert.NoError(t, err)

	_, err = ref.NewImage(context.Background(), nil)
	assert.Error(t, err)
}

func TestReferenceNewImageSource(t *testing.T) {
	ref, _ := refToTempDir(t)
	src, err := ref.NewImageSource(context.Background(), nil)
	assert.NoError(t, err)
	defer src.Close()
}

func TestReferenceNewImageDestination(t *testing.T) {
	ref, _ := refToTempDir(t)
	dest, err := ref.NewImageDestination(context.Background(), nil)
	assert.NoError(t, err)
	defer dest.Close()
}

func TestReferenceDeleteImage(t *testing.T) {
	ref, _ := refToTempDir(t)
	err := ref.DeleteImage(context.Background(), nil)
	assert.Error(t, err)
}

func TestReferenceManifestPath(t *testing.T) {
	dhex := digest.Digest("sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")

	ref, tmpDir := refToTempDir(t)
	dirRef, ok := ref.(dirReference)
	require.True(t, ok)
	res, err := dirRef.manifestPath(nil)
	require.NoError(t, err)
	assert.Equal(t, tmpDir+"/manifest.json", res)
	res, err = dirRef.manifestPath(&dhex)
	require.NoError(t, err)
	assert.Equal(t, tmpDir+"/"+dhex.Encoded()+".manifest.json", res)
	invalidDigest := digest.Digest("sha256:../hello")
	_, err = dirRef.manifestPath(&invalidDigest)
	assert.Error(t, err)
}

func TestReferenceLayerPath(t *testing.T) {
	const hex = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	ref, tmpDir := refToTempDir(t)
	dirRef, ok := ref.(dirReference)
	require.True(t, ok)
	res, err := dirRef.layerPath("sha256:" + hex)
	require.NoError(t, err)
	assert.Equal(t, tmpDir+"/"+hex, res)
	_, err = dirRef.layerPath(digest.Digest("sha256:../hello"))
	assert.Error(t, err)
}

func TestReferenceSignaturePath(t *testing.T) {
	dhex := digest.Digest("sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")

	ref, tmpDir := refToTempDir(t)
	dirRef, ok := ref.(dirReference)
	require.True(t, ok)
	res, err := dirRef.signaturePath(0, nil)
	require.NoError(t, err)
	assert.Equal(t, tmpDir+"/signature-1", res)
	res, err = dirRef.signaturePath(9, nil)
	require.NoError(t, err)
	assert.Equal(t, tmpDir+"/signature-10", res)
	res, err = dirRef.signaturePath(0, &dhex)
	require.NoError(t, err)
	assert.Equal(t, tmpDir+"/"+dhex.Encoded()+".signature-1", res)
	res, err = dirRef.signaturePath(9, &dhex)
	require.NoError(t, err)
	assert.Equal(t, tmpDir+"/"+dhex.Encoded()+".signature-10", res)
	invalidDigest := digest.Digest("sha256:../hello")
	_, err = dirRef.signaturePath(0, &invalidDigest)
	assert.Error(t, err)
}

func TestReferenceVersionPath(t *testing.T) {
	ref, tmpDir := refToTempDir(t)
	dirRef, ok := ref.(dirReference)
	require.True(t, ok)
	assert.Equal(t, tmpDir+"/version", dirRef.versionPath())
}

package layout

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/containers/image/v5/internal/testing/explicitfilepath-tmpdir"
	"github.com/containers/image/v5/types"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetManifestDescriptor(t *testing.T) {
	emptyDir := t.TempDir()

	for _, c := range []struct {
		dir, image         string
		expectedDescriptor *imgspecv1.Descriptor // nil if a failure ie expected. errorIs / errorAs allows more specific checks.
		expectedIndex      int
		errorIs            error
		errorAs            any
	}{
		{ // Index is missing
			dir:                emptyDir,
			image:              "",
			expectedDescriptor: nil,
		},
		{ // A valid reference to the only manifest
			dir:   "fixtures/manifest",
			image: "",
			expectedDescriptor: &imgspecv1.Descriptor{
				MediaType:   "application/vnd.oci.image.manifest.v1+json",
				Digest:      "sha256:84afb6189c4d69f2d040c5f1dc4e0a16fed9b539ce9cfb4ac2526ae4e0576cc0",
				Size:        496,
				Annotations: map[string]string{"org.opencontainers.image.ref.name": "v0.1.1"},
				Platform: &imgspecv1.Platform{
					Architecture: "amd64",
					OS:           "linux",
				},
			},
			expectedIndex: 0,
		},
		{ // An ambiguous reference to a multi-manifest directory
			dir:                "fixtures/two_images_manifest",
			image:              "",
			expectedDescriptor: nil,
			errorIs:            ErrMoreThanOneImage,
		},
		{ // A valid reference in a multi-manifest directory
			dir:   "fixtures/name_lookups",
			image: "a",
			expectedDescriptor: &imgspecv1.Descriptor{
				MediaType:   "application/vnd.oci.image.index.v1+json",
				Digest:      "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Size:        1,
				Annotations: map[string]string{"org.opencontainers.image.ref.name": "a"},
			},
			expectedIndex: 0,
		},
		{ // A valid reference in a multi-manifest directory
			dir:   "fixtures/name_lookups",
			image: "b",
			expectedDescriptor: &imgspecv1.Descriptor{
				MediaType:   "application/vnd.oci.image.manifest.v1+json",
				Digest:      "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				Size:        2,
				Annotations: map[string]string{"org.opencontainers.image.ref.name": "b"},
			},
			expectedIndex: 1,
		},
		{ // A valid reference in a multi-manifest directory
			dir:   "fixtures/name_lookups",
			image: "c",
			expectedDescriptor: &imgspecv1.Descriptor{
				MediaType:   "application/vnd.docker.distribution.manifest.list.v2+json",
				Digest:      "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
				Size:        3,
				Annotations: map[string]string{"org.opencontainers.image.ref.name": "c"},
			},
			expectedIndex: 2,
		},
		{ // A valid reference in a multi-manifest directory
			dir:   "fixtures/name_lookups",
			image: "d",
			expectedDescriptor: &imgspecv1.Descriptor{
				MediaType:   "application/vnd.docker.distribution.manifest.v2+json",
				Digest:      "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
				Size:        4,
				Annotations: map[string]string{"org.opencontainers.image.ref.name": "d"},
			},
			expectedIndex: 3,
		},
		{ // No entry found
			dir:                "fixtures/name_lookups",
			image:              "this-does-not-exist",
			expectedDescriptor: nil,
			errorAs:            &ImageNotFoundError{},
		},
		{ // Entries with invalid MIME types found
			dir:                "fixtures/name_lookups",
			image:              "invalid-mime",
			expectedDescriptor: nil,
		},
	} {
		ref, err := NewReference(c.dir, c.image)
		require.NoError(t, err)

		res, i, err := ref.(ociReference).getManifestDescriptor()
		if c.expectedDescriptor != nil {
			require.NoError(t, err)
			assert.Equal(t, c.expectedIndex, i)
			assert.Equal(t, *c.expectedDescriptor, res)
		} else {
			require.Error(t, err)
			if c.errorIs != nil {
				assert.ErrorIs(t, err, c.errorIs)
			}
			if c.errorAs != nil {
				assert.ErrorAs(t, err, &c.errorAs)
			}
		}
	}

	ref, err := NewIndexReference("fixtures/two_images_manifest", 0)
	assert.NoError(t, err)
	res, err := LoadManifestDescriptor(ref)
	assert.NoError(t, err)
	assert.Equal(t, imgspecv1.Descriptor{
		MediaType: "application/vnd.oci.image.manifest.v1+json",
		Digest:    "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f",
		Size:      7143,
		Platform: &imgspecv1.Platform{
			Architecture: "ppc64le",
			OS:           "linux",
		}}, res)

	// Out of bounds
	ref, err = NewIndexReference("fixtures/two_images_manifest", 6)
	assert.NoError(t, err)
	_, err = LoadManifestDescriptor(ref)
	assert.Error(t, err)
	assert.Equal(t, "index 6 is too large, only 2 entries available", err.Error())
}

func TestTransportName(t *testing.T) {
	assert.Equal(t, "oci", Transport.Name())
}

func TestTransportParseReference(t *testing.T) {
	testParseReference(t, Transport.ParseReference)
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
		"/",
		"/double//slashes",
		"/has/./dot",
		"/has/dot/../dot",
		"/trailing/slash/",
	} {
		err := Transport.ValidatePolicyConfigurationScope(scope)
		assert.Error(t, err, scope)
	}
}

func TestParseReference(t *testing.T) {
	testParseReference(t, ParseReference)
}

// testParseReference is a test shared for Transport.ParseReference and ParseReference.
func testParseReference(t *testing.T, fn func(string) (types.ImageReference, error)) {
	tmpDir := t.TempDir()

	for _, path := range []string{
		"/",
		"/etc",
		tmpDir,
		"relativepath",
		tmpDir + "/thisdoesnotexist",
	} {
		for _, image := range []struct {
			suffix, image string
			sourceIndex   int
		}{
			{":notlatest:image", "notlatest:image", -1},
			{":latestimage", "latestimage", -1},
			{":", "", -1},
			{"", "", -1},
			{":@0", "", 0},
			{":@10", "", 10},
			{":@999999", "", 999999},
		} {
			input := path + image.suffix
			ref, err := fn(input)
			require.NoError(t, err, input)
			ociRef, ok := ref.(ociReference)
			require.True(t, ok)
			assert.Equal(t, path, ociRef.dir, input)
			assert.Equal(t, image.image, ociRef.image, input)
			assert.Equal(t, image.sourceIndex, ociRef.sourceIndex, input)
		}
	}

	_, err := fn(tmpDir + ":invalid'image!value@")
	assert.Error(t, err)

	_, err = fn(tmpDir + ":@-3")
	assert.Error(t, err)
}

func TestNewReference(t *testing.T) {
	const (
		imageValue   = "imageValue"
		noImageValue = ""
	)

	tmpDir := t.TempDir()

	ref, err := NewReference(tmpDir, imageValue)
	require.NoError(t, err)
	ociRef, ok := ref.(ociReference)
	require.True(t, ok)
	assert.Equal(t, tmpDir, ociRef.dir)
	assert.Equal(t, imageValue, ociRef.image)
	assert.Equal(t, -1, ociRef.sourceIndex)

	ref, err = NewReference(tmpDir, noImageValue)
	require.NoError(t, err)
	ociRef, ok = ref.(ociReference)
	require.True(t, ok)
	assert.Equal(t, tmpDir, ociRef.dir)
	assert.Equal(t, noImageValue, ociRef.image)
	assert.Equal(t, -1, ociRef.sourceIndex)

	_, err = NewReference(tmpDir+"/thisparentdoesnotexist/something", imageValue)
	assert.Error(t, err)

	_, err = NewReference(tmpDir, "invalid'image!value@")
	assert.Error(t, err)

	_, err = NewReference(tmpDir+"/has:colon", imageValue)
	assert.Error(t, err)

	// Test private newReference
	_, err = newReference(tmpDir, imageValue, 1)
	assert.Error(t, err)
}

func TestNewIndexReference(t *testing.T) {
	const imageValue = "imageValue"

	tmpDir := t.TempDir()

	ref, err := NewIndexReference(tmpDir, 10)
	require.NoError(t, err)
	ociRef, ok := ref.(ociReference)
	require.True(t, ok)
	assert.Equal(t, tmpDir, ociRef.dir)
	assert.Equal(t, "", ociRef.image)
	assert.Equal(t, 10, ociRef.sourceIndex)

	ref, err = NewIndexReference(tmpDir, 9999)
	require.NoError(t, err)
	ociRef, ok = ref.(ociReference)
	require.True(t, ok)
	assert.Equal(t, tmpDir, ociRef.dir)
	assert.Equal(t, "", ociRef.image)
	assert.Equal(t, 9999, ociRef.sourceIndex)

	for _, c := range []struct {
		dir   string
		index int
	}{
		{tmpDir + "/thisparentdoesnotexist/something", 10},
		{tmpDir, -1},
		{tmpDir, -3},
		{tmpDir + "/has:colon", 99},
	} {
		_, err = NewIndexReference(c.dir, c.index)
		assert.Error(t, err)
	}

	// Test private newReference
	_, err = newReference(tmpDir, imageValue, 1)
	assert.Error(t, err)
	_, err = newReference(tmpDir, "", -3)
	assert.Error(t, err)
}

// refToTempOCI creates a temporary directory and returns an reference to it.
func refToTempOCI(t *testing.T, sourceIndex bool) (types.ImageReference, string) {
	tmpDir := t.TempDir()
	m := `{
		"schemaVersion": 2,
		"manifests": [
		{
			"mediaType": "application/vnd.oci.image.manifest.v1+json",
			"size": 7143,
			"digest": "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f",
			"platform": {
				"architecture": "ppc64le",
				"os": "linux"
			},
			"annotations": {
				"org.opencontainers.image.ref.name": "imageValue"
			}
		}
		]
	}
`
	if sourceIndex {
		m = `{
		"schemaVersion": 2,
		"manifests": [
		{
			"mediaType": "application/vnd.oci.image.manifest.v1+json",
			"size": 7143,
			"digest": "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f",
			"platform": {
				"architecture": "ppc64le",
				"os": "linux"
			},
		}
		]
	}
`
	}

	err := os.WriteFile(filepath.Join(tmpDir, "index.json"), []byte(m), 0644)
	require.NoError(t, err)
	var ref types.ImageReference
	if sourceIndex {
		ref, err = NewIndexReference(tmpDir, 1)
		require.NoError(t, err)
	} else {
		ref, err = NewReference(tmpDir, "imageValue")
		require.NoError(t, err)
	}
	return ref, tmpDir
}

func TestReferenceTransport(t *testing.T) {
	ref, _ := refToTempOCI(t, false)
	assert.Equal(t, Transport, ref.Transport())
}

func TestReferenceStringWithinTransport(t *testing.T) {
	tmpDir := t.TempDir()

	for _, c := range []struct{ input, result string }{
		{"/dir1:notlatest:notlatest", "/dir1:notlatest:notlatest"}, // Explicit image
		{"/dir3:", "/dir3:"},     // No image
		{"/dir4:@1", "/dir4:@1"}, // Explicit sourceIndex of image
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
	ref, _ := refToTempOCI(t, false)
	assert.Nil(t, ref.DockerReference())
}

func TestReferencePolicyConfigurationIdentity(t *testing.T) {
	ref, tmpDir := refToTempOCI(t, false)

	assert.Equal(t, tmpDir, ref.PolicyConfigurationIdentity())
	// A non-canonical path.  Test just one, the various other cases are
	// tested in explicitfilepath.ResolvePathToFullyExplicit.
	ref, err := NewReference(tmpDir+"/.", "image2")
	require.NoError(t, err)
	assert.Equal(t, tmpDir, ref.PolicyConfigurationIdentity())

	// Test the sourceIndex case
	ref, tmpDir = refToTempOCI(t, true)
	assert.Equal(t, tmpDir, ref.PolicyConfigurationIdentity())
	// A non-canonical path.  Test just one, the various other cases are
	// tested in explicitfilepath.ResolvePathToFullyExplicit.
	ref, err = NewIndexReference(tmpDir+"/.", 1)
	require.NoError(t, err)
	assert.Equal(t, tmpDir, ref.PolicyConfigurationIdentity())

	// "/" as a corner case.
	ref, err = NewReference("/", "image3")
	require.NoError(t, err)
	assert.Equal(t, "/", ref.PolicyConfigurationIdentity())

	ref, err = NewIndexReference("/", 2)
	require.NoError(t, err)
	assert.Equal(t, "/", ref.PolicyConfigurationIdentity())
}

func TestReferencePolicyConfigurationNamespaces(t *testing.T) {
	ref, tmpDir := refToTempOCI(t, false)
	// We don't really know enough to make a full equality test here.
	ns := ref.PolicyConfigurationNamespaces()
	require.NotNil(t, ns)
	assert.True(t, len(ns) >= 2)
	assert.Equal(t, tmpDir, ns[0])
	assert.Equal(t, filepath.Dir(tmpDir), ns[1])

	// Test the sourceIndex case
	ref, tmpDir = refToTempOCI(t, true)
	// We don't really know enough to make a full equality test here.
	ns = ref.PolicyConfigurationNamespaces()
	require.NotNil(t, ns)
	assert.True(t, len(ns) >= 2)
	assert.Equal(t, tmpDir, ns[0])
	assert.Equal(t, filepath.Dir(tmpDir), ns[1])

	// Test with a known path which should exist. Test just one non-canonical
	// path, the various other cases are tested in explicitfilepath.ResolvePathToFullyExplicit.
	//
	// It would be nice to test a deeper hierarchy, but it is not obvious what
	// deeper path is always available in the various distros, AND is not likely
	// to contains a symbolic link.
	for _, path := range []string{"/usr/share", "/usr/share/./."} {
		_, err := os.Lstat(path)
		require.NoError(t, err)
		ref, err := NewReference(path, "someimage")
		require.NoError(t, err)
		ns := ref.PolicyConfigurationNamespaces()
		require.NotNil(t, ns)
		assert.Equal(t, []string{"/usr/share", "/usr"}, ns)
	}

	// "/" as a corner case.
	ref, err := NewReference("/", "image3")
	require.NoError(t, err)
	assert.Equal(t, []string{}, ref.PolicyConfigurationNamespaces())

	ref, err = NewIndexReference("/", 2)
	require.NoError(t, err)
	assert.Equal(t, []string{}, ref.PolicyConfigurationNamespaces())
}

func TestReferenceNewImage(t *testing.T) {
	ref, _ := refToTempOCI(t, false)
	_, err := ref.NewImage(context.Background(), nil)
	assert.Error(t, err)
}

func TestReferenceNewImageSource(t *testing.T) {
	ref, _ := refToTempOCI(t, false)
	src, err := ref.NewImageSource(context.Background(), nil)
	assert.NoError(t, err)
	defer src.Close()
}

func TestReferenceNewImageDestination(t *testing.T) {
	ref, _ := refToTempOCI(t, false)
	dest, err := ref.NewImageDestination(context.Background(), nil)
	assert.NoError(t, err)
	defer dest.Close()
}

func TestReferenceOCILayoutPath(t *testing.T) {
	ref, tmpDir := refToTempOCI(t, false)
	ociRef, ok := ref.(ociReference)
	require.True(t, ok)
	assert.Equal(t, tmpDir+"/oci-layout", ociRef.ociLayoutPath())
}

func TestReferenceIndexPath(t *testing.T) {
	ref, tmpDir := refToTempOCI(t, false)
	ociRef, ok := ref.(ociReference)
	require.True(t, ok)
	assert.Equal(t, tmpDir+"/index.json", ociRef.indexPath())
}

func TestReferenceBlobPath(t *testing.T) {
	const hex = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	ref, tmpDir := refToTempOCI(t, false)
	ociRef, ok := ref.(ociReference)
	require.True(t, ok)
	bp, err := ociRef.blobPath("sha256:"+hex, "")
	assert.NoError(t, err)
	assert.Equal(t, tmpDir+"/blobs/sha256/"+hex, bp)
}

func TestReferenceSharedBlobPathShared(t *testing.T) {
	const hex = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	ref, _ := refToTempOCI(t, false)
	ociRef, ok := ref.(ociReference)
	require.True(t, ok)
	bp, err := ociRef.blobPath("sha256:"+hex, "/external/path")
	assert.NoError(t, err)
	assert.Equal(t, "/external/path/sha256/"+hex, bp)
}

func TestReferenceBlobPathInvalid(t *testing.T) {
	const hex = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	ref, _ := refToTempOCI(t, false)
	ociRef, ok := ref.(ociReference)
	require.True(t, ok)
	_, err := ociRef.blobPath(hex, "")
	assert.ErrorContains(t, err, "unexpected digest reference "+hex)
}

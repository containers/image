package image

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/internal/testing/mocks"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/types"
	"github.com/opencontainers/go-digest"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"
)

const commonFixtureConfigDigest = "sha256:9ca4bda0a6b3727a6ffcc43e981cad0f24e2ec79d338f6ba325b4dfd0756fb8f"

func manifestSchema2FromFixture(t *testing.T, src types.ImageSource, fixture string, mustFail bool) genericManifest {
	manifest, err := os.ReadFile(filepath.Join("fixtures", fixture))
	require.NoError(t, err)

	m, err := manifestSchema2FromManifest(src, manifest)
	if mustFail {
		require.Error(t, err)
	} else {
		require.NoError(t, err)
	}
	return m
}

func manifestSchema2FromComponentsLikeFixture(configBlob []byte) genericManifest {
	return manifestSchema2FromComponents(manifest.Schema2Descriptor{
		MediaType: "application/octet-stream",
		Size:      5940,
		Digest:    commonFixtureConfigDigest,
	}, nil, configBlob, []manifest.Schema2Descriptor{
		{
			MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip",
			Digest:    "sha256:6a5a5368e0c2d3e5909184fa28ddfd56072e7ff3ee9a945876f7eee5896ef5bb",
			Size:      51354364,
		},
		{
			MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip",
			Digest:    "sha256:1bbf5d58d24c47512e234a5623474acf65ae00d4d1414272a893204f44cc680c",
			Size:      150,
		},
		{
			MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip",
			Digest:    "sha256:8f5dc8a4b12c307ac84de90cdd9a7f3915d1be04c9388868ca118831099c67a9",
			Size:      11739507,
		},
		{
			MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip",
			Digest:    "sha256:bbd6b22eb11afce63cc76f6bc41042d99f10d6024c96b655dafba930b8d25909",
			Size:      8841833,
		},
		{
			MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip",
			Digest:    "sha256:960e52ecf8200cbd84e70eb2ad8678f4367e50d14357021872c10fa3fc5935fa",
			Size:      291,
		},
	})
}

func TestManifestSchema2FromManifest(t *testing.T) {
	// This just tests that the JSON can be loaded; we test that the parsed
	// values are correctly returned in tests for the individual getter methods.
	_ = manifestSchema2FromFixture(t, mocks.ForbiddenImageSource{}, "schema2.json", false)

	_, err := manifestSchema2FromManifest(nil, []byte{})
	assert.Error(t, err)
}

func TestManifestSchema2FromComponents(t *testing.T) {
	// This just smoke-tests that the manifest can be created; we test that the parsed
	// values are correctly returned in tests for the individual getter methods.
	_ = manifestSchema2FromComponentsLikeFixture(nil)
}

func TestManifestSchema2Serialize(t *testing.T) {
	for _, m := range []genericManifest{
		manifestSchema2FromFixture(t, mocks.ForbiddenImageSource{}, "schema2.json", false),
		manifestSchema2FromComponentsLikeFixture(nil),
	} {
		serialized, err := m.serialize()
		require.NoError(t, err)
		// We would ideally like to compare “serialized” with some transformation of
		// the original fixture, but the ordering of fields in JSON maps is undefined, so this is
		// easier.
		assertJSONEqualsFixture(t, serialized, "schema2.json")
	}
}

func TestManifestSchema2ManifestMIMEType(t *testing.T) {
	for _, m := range []genericManifest{
		manifestSchema2FromFixture(t, mocks.ForbiddenImageSource{}, "schema2.json", false),
		manifestSchema2FromComponentsLikeFixture(nil),
	} {
		assert.Equal(t, manifest.DockerV2Schema2MediaType, m.manifestMIMEType())
	}
}

func TestManifestSchema2ConfigInfo(t *testing.T) {
	for _, m := range []genericManifest{
		manifestSchema2FromFixture(t, mocks.ForbiddenImageSource{}, "schema2.json", false),
		manifestSchema2FromComponentsLikeFixture(nil),
	} {
		assert.Equal(t, types.BlobInfo{
			Size:      5940,
			Digest:    commonFixtureConfigDigest,
			MediaType: "application/octet-stream",
		}, m.ConfigInfo())
	}
}

// configBlobImageSource allows testing various GetBlob behaviors in .ConfigBlob()
type configBlobImageSource struct {
	mocks.ForbiddenImageSource // We inherit almost all of the methods, which just panic()
	expectedDigest             digest.Digest
	f                          func() (io.ReadCloser, int64, error)
}

func (f configBlobImageSource) GetBlob(ctx context.Context, info types.BlobInfo, _ types.BlobInfoCache) (io.ReadCloser, int64, error) {
	if info.Digest != f.expectedDigest {
		panic("Unexpected digest in GetBlob")
	}
	return f.f()
}

func TestManifestSchema2ConfigBlob(t *testing.T) {
	realConfigJSON, err := os.ReadFile("fixtures/schema2-config.json")
	require.NoError(t, err)

	for _, c := range []struct {
		cbISfn func() (io.ReadCloser, int64, error)
		blob   []byte
	}{
		// Success
		{func() (io.ReadCloser, int64, error) {
			return io.NopCloser(bytes.NewReader(realConfigJSON)), int64(len(realConfigJSON)), nil
		}, realConfigJSON},
		// Various kinds of failures
		{nil, nil},
		{func() (io.ReadCloser, int64, error) {
			return nil, -1, errors.New("Error returned from GetBlob")
		}, nil},
		{func() (io.ReadCloser, int64, error) {
			reader, writer := io.Pipe()
			err = writer.CloseWithError(errors.New("Expected error reading input in ConfigBlob"))
			assert.NoError(t, err)
			return reader, 1, nil
		}, nil},
		{func() (io.ReadCloser, int64, error) {
			nonmatchingJSON := []byte("This does not match ConfigDescriptor.Digest")
			return io.NopCloser(bytes.NewReader(nonmatchingJSON)), int64(len(nonmatchingJSON)), nil
		}, nil},
	} {
		var src types.ImageSource
		if c.cbISfn != nil {
			src = configBlobImageSource{
				expectedDigest: commonFixtureConfigDigest,
				f:              c.cbISfn,
			}
		} else {
			src = nil
		}
		m := manifestSchema2FromFixture(t, src, "schema2.json", false)
		blob, err := m.ConfigBlob(context.Background())
		if c.blob != nil {
			assert.NoError(t, err)
			assert.Equal(t, c.blob, blob)
		} else {
			assert.Error(t, err)
		}
	}

	// Generally configBlob should match ConfigInfo; we don’t quite need it to, and this will
	// guarantee that the returned object is returning the original contents instead
	// of reading an object from elsewhere.
	configBlob := []byte("config blob which does not match ConfigInfo")
	// This just tests that the manifest can be created; we test that the parsed
	// values are correctly returned in tests for the individual getter methods.
	m := manifestSchema2FromComponentsLikeFixture(configBlob)
	cb, err := m.ConfigBlob(context.Background())
	require.NoError(t, err)
	assert.Equal(t, configBlob, cb)
}

func TestManifestSchema2LayerInfo(t *testing.T) {
	for _, m := range []genericManifest{
		manifestSchema2FromFixture(t, mocks.ForbiddenImageSource{}, "schema2.json", false),
		manifestSchema2FromComponentsLikeFixture(nil),
	} {
		assert.Equal(t, []types.BlobInfo{
			{
				Digest:    "sha256:6a5a5368e0c2d3e5909184fa28ddfd56072e7ff3ee9a945876f7eee5896ef5bb",
				Size:      51354364,
				MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip",
			},
			{
				Digest:    "sha256:1bbf5d58d24c47512e234a5623474acf65ae00d4d1414272a893204f44cc680c",
				Size:      150,
				MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip",
			},
			{
				Digest:    "sha256:8f5dc8a4b12c307ac84de90cdd9a7f3915d1be04c9388868ca118831099c67a9",
				Size:      11739507,
				MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip",
			},
			{
				Digest:    "sha256:bbd6b22eb11afce63cc76f6bc41042d99f10d6024c96b655dafba930b8d25909",
				Size:      8841833,
				MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip",
			},
			{
				Digest:    "sha256:960e52ecf8200cbd84e70eb2ad8678f4367e50d14357021872c10fa3fc5935fa",
				Size:      291,
				MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip",
			},
		}, m.LayerInfos())
	}
}

func TestManifestSchema2EmbeddedDockerReferenceConflicts(t *testing.T) {
	for _, m := range []genericManifest{
		manifestSchema2FromFixture(t, mocks.ForbiddenImageSource{}, "schema2.json", false),
		manifestSchema2FromComponentsLikeFixture(nil),
	} {
		for _, name := range []string{"busybox", "example.com:5555/ns/repo:tag"} {
			ref, err := reference.ParseNormalizedNamed(name)
			require.NoError(t, err)
			conflicts := m.EmbeddedDockerReferenceConflicts(ref)
			assert.False(t, conflicts)
		}
	}
}

func TestManifestSchema2Inspect(t *testing.T) {
	configJSON, err := os.ReadFile("fixtures/schema2-config.json")
	require.NoError(t, err)

	m := manifestSchema2FromComponentsLikeFixture(configJSON)
	ii, err := m.Inspect(context.Background())
	require.NoError(t, err)
	created := time.Date(2016, 9, 23, 23, 20, 45, 789764590, time.UTC)

	var emptyAnnotations map[string]string
	assert.Equal(t, types.ImageInspectInfo{
		Tag:           "",
		Created:       &created,
		DockerVersion: "1.12.1",
		Labels:        map[string]string{},
		Architecture:  "amd64",
		Os:            "linux",
		Layers: []string{
			"sha256:6a5a5368e0c2d3e5909184fa28ddfd56072e7ff3ee9a945876f7eee5896ef5bb",
			"sha256:1bbf5d58d24c47512e234a5623474acf65ae00d4d1414272a893204f44cc680c",
			"sha256:8f5dc8a4b12c307ac84de90cdd9a7f3915d1be04c9388868ca118831099c67a9",
			"sha256:bbd6b22eb11afce63cc76f6bc41042d99f10d6024c96b655dafba930b8d25909",
			"sha256:960e52ecf8200cbd84e70eb2ad8678f4367e50d14357021872c10fa3fc5935fa",
		},
		LayersData: []types.ImageInspectLayer{{
			MIMEType:    "application/vnd.docker.image.rootfs.diff.tar.gzip",
			Digest:      "sha256:6a5a5368e0c2d3e5909184fa28ddfd56072e7ff3ee9a945876f7eee5896ef5bb",
			Size:        51354364,
			Annotations: emptyAnnotations,
		}, {
			MIMEType:    "application/vnd.docker.image.rootfs.diff.tar.gzip",
			Digest:      "sha256:1bbf5d58d24c47512e234a5623474acf65ae00d4d1414272a893204f44cc680c",
			Size:        150,
			Annotations: emptyAnnotations,
		}, {
			MIMEType:    "application/vnd.docker.image.rootfs.diff.tar.gzip",
			Digest:      "sha256:8f5dc8a4b12c307ac84de90cdd9a7f3915d1be04c9388868ca118831099c67a9",
			Size:        11739507,
			Annotations: emptyAnnotations,
		}, {
			MIMEType:    "application/vnd.docker.image.rootfs.diff.tar.gzip",
			Digest:      "sha256:bbd6b22eb11afce63cc76f6bc41042d99f10d6024c96b655dafba930b8d25909",
			Size:        8841833,
			Annotations: emptyAnnotations,
		}, {
			MIMEType:    "application/vnd.docker.image.rootfs.diff.tar.gzip",
			Digest:      "sha256:960e52ecf8200cbd84e70eb2ad8678f4367e50d14357021872c10fa3fc5935fa",
			Size:        291,
			Annotations: emptyAnnotations,
		},
		},
		Author: "",
		Env: []string{
			"PATH=/usr/local/apache2/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			"HTTPD_PREFIX=/usr/local/apache2",
			"HTTPD_VERSION=2.4.23",
			"HTTPD_SHA1=5101be34ac4a509b245adb70a56690a84fcc4e7f",
			"HTTPD_BZ2_URL=https://www.apache.org/dyn/closer.cgi?action=download&filename=httpd/httpd-2.4.23.tar.bz2",
			"HTTPD_ASC_URL=https://www.apache.org/dist/httpd/httpd-2.4.23.tar.bz2.asc",
		},
	}, *ii)

	// nil configBlob will trigger an error in m.ConfigBlob()
	m = manifestSchema2FromComponentsLikeFixture(nil)
	_, err = m.Inspect(context.Background())
	assert.Error(t, err)

	m = manifestSchema2FromComponentsLikeFixture([]byte("invalid JSON"))
	_, err = m.Inspect(context.Background())
	assert.Error(t, err)
}

func TestManifestSchema2UpdatedImageNeedsLayerDiffIDs(t *testing.T) {
	for _, m := range []genericManifest{
		manifestSchema2FromFixture(t, mocks.ForbiddenImageSource{}, "schema2.json", false),
		manifestSchema2FromComponentsLikeFixture(nil),
	} {
		assert.False(t, m.UpdatedImageNeedsLayerDiffIDs(types.ManifestUpdateOptions{
			ManifestMIMEType: manifest.DockerV2Schema1SignedMediaType,
		}))
	}
}

// schema2ImageSource is plausible enough for schema conversions in manifestSchema2.UpdatedImage() to work.
type schema2ImageSource struct {
	configBlobImageSource
	ref reference.Named
}

func (s2is *schema2ImageSource) Reference() types.ImageReference {
	return refImageReferenceMock{ref: s2is.ref}
}

// refImageReferenceMock is a mock of types.ImageReference which returns itself in DockerReference.
type refImageReferenceMock struct {
	mocks.ForbiddenImageReference // We inherit almost all of the methods, which just panic()
	ref                           reference.Named
}

func (ref refImageReferenceMock) DockerReference() reference.Named {
	return ref.ref
}

func newSchema2ImageSource(t *testing.T, dockerRef string) *schema2ImageSource {
	realConfigJSON, err := os.ReadFile("fixtures/schema2-config.json")
	require.NoError(t, err)

	ref, err := reference.ParseNormalizedNamed(dockerRef)
	require.NoError(t, err)

	return &schema2ImageSource{
		configBlobImageSource: configBlobImageSource{
			expectedDigest: commonFixtureConfigDigest,
			f: func() (io.ReadCloser, int64, error) {
				return io.NopCloser(bytes.NewReader(realConfigJSON)), int64(len(realConfigJSON)), nil
			},
		},
		ref: ref,
	}
}

type memoryImageDest struct {
	ref         reference.Named
	storedBlobs map[digest.Digest][]byte
}

func (d *memoryImageDest) Reference() types.ImageReference {
	return refImageReferenceMock{ref: d.ref}
}
func (d *memoryImageDest) Close() error {
	panic("Unexpected call to a mock function")
}
func (d *memoryImageDest) SupportedManifestMIMETypes() []string {
	panic("Unexpected call to a mock function")
}
func (d *memoryImageDest) SupportsSignatures(ctx context.Context) error {
	panic("Unexpected call to a mock function")
}
func (d *memoryImageDest) DesiredLayerCompression() types.LayerCompression {
	panic("Unexpected call to a mock function")
}
func (d *memoryImageDest) AcceptsForeignLayerURLs() bool {
	panic("Unexpected call to a mock function")
}
func (d *memoryImageDest) MustMatchRuntimeOS() bool {
	panic("Unexpected call to a mock function")
}
func (d *memoryImageDest) IgnoresEmbeddedDockerReference() bool {
	panic("Unexpected call to a mock function")
}
func (d *memoryImageDest) HasThreadSafePutBlob() bool {
	panic("Unexpected call to a mock function")
}
func (d *memoryImageDest) PutBlob(ctx context.Context, stream io.Reader, inputInfo types.BlobInfo, cache types.BlobInfoCache, isConfig bool) (types.BlobInfo, error) {
	if d.storedBlobs == nil {
		d.storedBlobs = make(map[digest.Digest][]byte)
	}
	if inputInfo.Digest == "" {
		panic("inputInfo.Digest unexpectedly empty")
	}
	contents, err := io.ReadAll(stream)
	if err != nil {
		return types.BlobInfo{}, err
	}
	d.storedBlobs[inputInfo.Digest] = contents
	return types.BlobInfo{Digest: inputInfo.Digest, Size: int64(len(contents))}, nil
}
func (d *memoryImageDest) TryReusingBlob(context.Context, types.BlobInfo, types.BlobInfoCache, bool) (bool, types.BlobInfo, error) {
	panic("Unexpected call to a mock function")
}
func (d *memoryImageDest) PutManifest(context.Context, []byte, *digest.Digest) error {
	panic("Unexpected call to a mock function")
}
func (d *memoryImageDest) PutSignatures(ctx context.Context, signatures [][]byte, instanceDigest *digest.Digest) error {
	panic("Unexpected call to a mock function")
}
func (d *memoryImageDest) Commit(context.Context, types.UnparsedImage) error {
	panic("Unexpected call to a mock function")
}

// modifiedLayerInfos returns two identical (but separately allocated) copies of
// layers from input, where the size and digest of each item is predictably modified from the original in input.
// (This is used to test ManifestUpdateOptions.LayerInfos handling.)
func modifiedLayerInfos(t *testing.T, input []types.BlobInfo) ([]types.BlobInfo, []types.BlobInfo) {
	modified := []types.BlobInfo{}
	for _, blob := range input {
		b2 := blob
		oldDigest, err := hex.DecodeString(b2.Digest.Encoded())
		require.NoError(t, err)
		oldDigest[len(oldDigest)-1] ^= 1
		b2.Digest = digest.NewDigestFromEncoded(b2.Digest.Algorithm(), hex.EncodeToString(oldDigest))
		b2.Size ^= 1
		modified = append(modified, b2)
	}

	copy := slices.Clone(modified)
	return modified, copy
}

func TestManifestSchema2UpdatedImage(t *testing.T) {
	originalSrc := newSchema2ImageSource(t, "httpd:latest")
	original := manifestSchema2FromFixture(t, originalSrc, "schema2.json", false)

	// LayerInfos:
	layerInfos := append(original.LayerInfos()[1:], original.LayerInfos()[0])
	res, err := original.UpdatedImage(context.Background(), types.ManifestUpdateOptions{
		LayerInfos: layerInfos,
	})
	require.NoError(t, err)
	assert.Equal(t, layerInfos, res.LayerInfos())
	_, err = original.UpdatedImage(context.Background(), types.ManifestUpdateOptions{
		LayerInfos: append(layerInfos, layerInfos[0]),
	})
	assert.Error(t, err)

	// EmbeddedDockerReference:
	// … is ignored
	embeddedRef, err := reference.ParseNormalizedNamed("busybox")
	require.NoError(t, err)
	res, err = original.UpdatedImage(context.Background(), types.ManifestUpdateOptions{
		EmbeddedDockerReference: embeddedRef,
	})
	require.NoError(t, err)
	nonEmbeddedRef, err := reference.ParseNormalizedNamed("notbusybox:notlatest")
	require.NoError(t, err)
	conflicts := res.EmbeddedDockerReferenceConflicts(nonEmbeddedRef)
	assert.False(t, conflicts)

	// ManifestMIMEType:
	// Only smoke-test the valid conversions, detailed tests are below. (This also verifies that “original” is not affected.)
	for _, mime := range []string{
		manifest.DockerV2Schema1MediaType,
		manifest.DockerV2Schema1SignedMediaType,
	} {
		_, err = original.UpdatedImage(context.Background(), types.ManifestUpdateOptions{
			ManifestMIMEType: mime,
			InformationOnly: types.ManifestUpdateInformation{
				Destination: &memoryImageDest{ref: originalSrc.ref},
			},
		})
		assert.NoError(t, err, mime)
	}
	for _, mime := range []string{
		manifest.DockerV2Schema2MediaType, // This indicates a confused caller, not a no-op
		"this is invalid",
	} {
		_, err = original.UpdatedImage(context.Background(), types.ManifestUpdateOptions{
			ManifestMIMEType: mime,
		})
		assert.Error(t, err, mime)
	}

	// m hasn’t been changed:
	m2 := manifestSchema2FromFixture(t, originalSrc, "schema2.json", false)
	typedOriginal, ok := original.(*manifestSchema2)
	require.True(t, ok)
	typedM2, ok := m2.(*manifestSchema2)
	require.True(t, ok)
	assert.Equal(t, *typedM2, *typedOriginal)
}

func TestConvertToManifestOCI(t *testing.T) {
	originalSrc := newSchema2ImageSource(t, "httpd-copy:latest")
	original := manifestSchema2FromFixture(t, originalSrc, "schema2.json", false)
	res, err := original.UpdatedImage(context.Background(), types.ManifestUpdateOptions{
		ManifestMIMEType: imgspecv1.MediaTypeImageManifest,
	})
	require.NoError(t, err)

	convertedJSON, mt, err := res.Manifest(context.Background())
	require.NoError(t, err)
	assert.Equal(t, imgspecv1.MediaTypeImageManifest, mt)
	assertJSONEqualsFixture(t, convertedJSON, "schema2-to-oci1.json")

	convertedConfig, err := res.ConfigBlob(context.Background())
	require.NoError(t, err)
	assertJSONEqualsFixture(t, convertedConfig, "schema2-to-oci1-config.json")
}

func TestConvertToManifestOCIAllMediaTypes(t *testing.T) {
	originalSrc := newSchema2ImageSource(t, "httpd-copy:latest")
	original := manifestSchema2FromFixture(t, originalSrc, "schema2-all-media-types.json", false)
	res, err := original.UpdatedImage(context.Background(), types.ManifestUpdateOptions{
		ManifestMIMEType: imgspecv1.MediaTypeImageManifest,
	})
	require.NoError(t, err)
	convertedJSON, mt, err := res.Manifest(context.Background())
	require.NoError(t, err)
	assert.Equal(t, imgspecv1.MediaTypeImageManifest, mt)
	assertJSONEqualsFixture(t, convertedJSON, "schema2-all-media-types-to-oci1.json")

	convertedConfig, err := res.ConfigBlob(context.Background())
	require.NoError(t, err)
	assertJSONEqualsFixture(t, convertedConfig, "schema2-to-oci1-config.json")
}

func TestConvertToOCIWithInvalidMIMEType(t *testing.T) {
	originalSrc := newSchema2ImageSource(t, "httpd-copy:latest")
	manifestSchema2FromFixture(t, originalSrc, "schema2-invalid-media-type.json", true)
}

func TestConvertToManifestSchema1(t *testing.T) {
	originalSrc := newSchema2ImageSource(t, "httpd-copy:latest")
	original := manifestSchema2FromFixture(t, originalSrc, "schema2.json", false)
	memoryDest := &memoryImageDest{ref: originalSrc.ref}
	res, err := original.UpdatedImage(context.Background(), types.ManifestUpdateOptions{
		ManifestMIMEType: manifest.DockerV2Schema1SignedMediaType,
		InformationOnly: types.ManifestUpdateInformation{
			Destination: memoryDest,
		},
	})
	require.NoError(t, err)

	convertedJSON, mt, err := res.Manifest(context.Background())
	require.NoError(t, err)
	assert.Equal(t, manifest.DockerV2Schema1SignedMediaType, mt)

	// schema2-to-schema1-by-docker.json is the result of asking the Docker Hub for a schema1 manifest,
	// except that we have replaced "name" to verify that the ref from
	// memoryDest, not from originalSrc, is used.
	assertJSONEqualsFixture(t, convertedJSON, "schema2-to-schema1-by-docker.json", "signatures")

	assert.Equal(t, GzippedEmptyLayer, memoryDest.storedBlobs[GzippedEmptyLayerDigest])

	// Conversion to schema1 together with changing LayerInfos works as expected (which requires
	// handling schema1 empty layers):
	updatedLayers, updatedLayersCopy := modifiedLayerInfos(t, original.LayerInfos())
	res, err = original.UpdatedImage(context.Background(), types.ManifestUpdateOptions{
		LayerInfos:       updatedLayers,
		ManifestMIMEType: manifest.DockerV2Schema1SignedMediaType,
		InformationOnly: types.ManifestUpdateInformation{
			Destination: memoryDest,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, updatedLayersCopy, updatedLayers) // updatedLayers have not been modified in place
	convertedJSON, mt, err = res.Manifest(context.Background())
	require.NoError(t, err)
	assert.Equal(t, manifest.DockerV2Schema1SignedMediaType, mt)
	// Layers have been updated as expected
	s1Manifest, err := manifestSchema1FromManifest(convertedJSON)
	require.NoError(t, err)
	assert.Equal(t, []types.BlobInfo{
		{Digest: "sha256:6a5a5368e0c2d3e5909184fa28ddfd56072e7ff3ee9a945876f7eee5896ef5ba", Size: -1},
		{Digest: GzippedEmptyLayerDigest, Size: -1},
		{Digest: GzippedEmptyLayerDigest, Size: -1},
		{Digest: GzippedEmptyLayerDigest, Size: -1},
		{Digest: "sha256:1bbf5d58d24c47512e234a5623474acf65ae00d4d1414272a893204f44cc680d", Size: -1},
		{Digest: GzippedEmptyLayerDigest, Size: -1},
		{Digest: "sha256:8f5dc8a4b12c307ac84de90cdd9a7f3915d1be04c9388868ca118831099c67a8", Size: -1},
		{Digest: GzippedEmptyLayerDigest, Size: -1},
		{Digest: GzippedEmptyLayerDigest, Size: -1},
		{Digest: GzippedEmptyLayerDigest, Size: -1},
		{Digest: GzippedEmptyLayerDigest, Size: -1},
		{Digest: "sha256:bbd6b22eb11afce63cc76f6bc41042d99f10d6024c96b655dafba930b8d25908", Size: -1},
		{Digest: "sha256:960e52ecf8200cbd84e70eb2ad8678f4367e50d14357021872c10fa3fc5935fb", Size: -1},
		{Digest: GzippedEmptyLayerDigest, Size: -1},
		{Digest: GzippedEmptyLayerDigest, Size: -1},
	}, s1Manifest.LayerInfos())

	// FIXME? Test also the various failure cases, if only to see that we don't crash?
}

func TestConvertSchema2ToManifestOCIWithAnnotations(t *testing.T) {
	// Test when converting an image from schema 2 (which doesn't support certain fields like
	// URLs, annotations, etc.) to an OCI image (which supports those fields),
	// that UpdatedImage propagates the features to the converted manifest.
	originalSrc := newSchema2ImageSource(t, "httpd-copy:latest")
	original := manifestSchema2FromFixture(t, originalSrc, "schema2.json", false)
	layerInfoOverwrites := []types.BlobInfo{
		{
			Digest:    "sha256:6a5a5368e0c2d3e5909184fa28ddfd56072e7ff3ee9a945876f7eee5896ef5bb",
			Size:      51354364,
			MediaType: imgspecv1.MediaTypeImageLayerGzip,
		},
		{
			Digest:    "sha256:1bbf5d58d24c47512e234a5623474acf65ae00d4d1414272a893204f44cc680c",
			Size:      150,
			MediaType: imgspecv1.MediaTypeImageLayerGzip,
		},
		{
			Digest: "sha256:8f5dc8a4b12c307ac84de90cdd9a7f3915d1be04c9388868ca118831099c67a9",
			Size:   11739507,
			URLs: []string{
				"https://layer.url",
			},
			MediaType: imgspecv1.MediaTypeImageLayerGzip,
		},
		{
			Digest: "sha256:bbd6b22eb11afce63cc76f6bc41042d99f10d6024c96b655dafba930b8d25909",
			Size:   8841833,
			Annotations: map[string]string{
				"test-annotation-2": "two",
			},
			MediaType: imgspecv1.MediaTypeImageLayerGzip,
		},
		{
			Digest:    "sha256:960e52ecf8200cbd84e70eb2ad8678f4367e50d14357021872c10fa3fc5935fa",
			Size:      291,
			MediaType: imgspecv1.MediaTypeImageLayerGzip,
		},
	}
	res, err := original.UpdatedImage(context.Background(), types.ManifestUpdateOptions{
		ManifestMIMEType: imgspecv1.MediaTypeImageManifest,
		LayerInfos:       layerInfoOverwrites,
	})
	require.NoError(t, err)
	assert.Equal(t, res.LayerInfos(), layerInfoOverwrites)

	// Doing this with schema2 should fail
	originalSrc = newSchema2ImageSource(t, "httpd-copy:latest")
	original = manifestSchema2FromFixture(t, originalSrc, "schema2.json", false)
	res, err = original.UpdatedImage(context.Background(), types.ManifestUpdateOptions{
		ManifestMIMEType: "",
		LayerInfos:       layerInfoOverwrites,
	})
	require.NoError(t, err)
	assert.NotEqual(t, res.LayerInfos(), layerInfoOverwrites)
}

func TestManifestSchema2CanChangeLayerCompression(t *testing.T) {
	for _, m := range []genericManifest{
		manifestSchema2FromFixture(t, mocks.ForbiddenImageSource{}, "schema2.json", false),
		manifestSchema2FromComponentsLikeFixture(nil),
	} {
		assert.True(t, m.CanChangeLayerCompression(manifest.DockerV2Schema2LayerMediaType))
		// Some projects like to use squashfs and other unspecified formats for layers; don’t touch those.
		assert.False(t, m.CanChangeLayerCompression("a completely unknown and quite possibly invalid MIME type"))
	}
}

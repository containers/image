package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/containers/image/v5/pkg/compression"
	"github.com/containers/image/v5/types"
	"github.com/opencontainers/go-digest"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func manifestOCI1FromFixture(t *testing.T, fixture string) *OCI1 {
	manifest, err := os.ReadFile(filepath.Join("fixtures", fixture))
	require.NoError(t, err)

	m, err := OCI1FromManifest(manifest)
	require.NoError(t, err)
	return m
}

func TestSupportedOCI1MediaType(t *testing.T) {
	type testData struct {
		m        string
		mustFail bool
	}
	data := []testData{
		{imgspecv1.MediaTypeDescriptor, false},
		{imgspecv1.MediaTypeImageConfig, false},
		{imgspecv1.MediaTypeImageLayer, false},
		{imgspecv1.MediaTypeImageLayerGzip, false},
		{imgspecv1.MediaTypeImageLayerNonDistributable, false},     //nolint:staticcheck // NonDistributable layers are deprecated, but we want to continue to support manipulating pre-existing images.
		{imgspecv1.MediaTypeImageLayerNonDistributableGzip, false}, //nolint:staticcheck // NonDistributable layers are deprecated, but we want to continue to support manipulating pre-existing images.
		{imgspecv1.MediaTypeImageLayerNonDistributableZstd, false}, //nolint:staticcheck // NonDistributable layers are deprecated, but we want to continue to support manipulating pre-existing images.
		{imgspecv1.MediaTypeImageLayerZstd, false},
		{imgspecv1.MediaTypeImageManifest, false},
		{imgspecv1.MediaTypeLayoutHeader, false},
		{"application/vnd.oci.image.layer.nondistributable.v1.tar+unknown", true},
	}
	for _, d := range data {
		err := SupportedOCI1MediaType(d.m)
		if d.mustFail {
			assert.NotNil(t, err)
		} else {
			assert.Nil(t, err)
		}
	}
}

func TestOCI1FromManifest(t *testing.T) {
	validManifest, err := os.ReadFile(filepath.Join("fixtures", "ociv1.manifest.json"))
	require.NoError(t, err)

	parser := func(m []byte) error {
		_, err := OCI1FromManifest(m)
		return err
	}
	// Schema mismatch is rejected
	testManifestFixturesAreRejected(t, parser, []string{
		"schema2-to-schema1-by-docker.json",
		// Not "v2s2.manifest.json" yet, without mediaType the two are too similar to tell the difference.
		"v2list.manifest.json",
		"ociv1.image.index.json",
	})
	// Extra fields are rejected
	testValidManifestWithExtraFieldsIsRejected(t, parser, validManifest, []string{"fsLayers", "history", "manifests"})
}

func TestOCI1UpdateLayerInfos(t *testing.T) {
	customCompression := compression.Algorithm{}

	for _, c := range []struct {
		name            string
		sourceFixture   string
		updates         []types.BlobInfo
		expectedFixture string // or "" to indicate an expected failure
	}{
		{
			name:          "gzip → zstd",
			sourceFixture: "ociv1.manifest.json",
			updates: []types.BlobInfo{
				{
					Digest:               "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f",
					Size:                 32654,
					MediaType:            imgspecv1.MediaTypeImageLayerGzip,
					CompressionOperation: types.Compress,
					CompressionAlgorithm: &compression.Zstd,
				},
				{
					Digest:               "sha256:3c3a4604a545cdc127456d94e421cd355bca5b528f4a9c1905b15da2eb4a4c6b",
					Size:                 16724,
					MediaType:            imgspecv1.MediaTypeImageLayerGzip,
					CompressionOperation: types.Compress,
					CompressionAlgorithm: &compression.Zstd,
				},
				{
					Digest:               "sha256:ec4b8955958665577945c89419d1af06b5f7636b4ac3da7f12184802ad867736",
					Size:                 73109,
					MediaType:            imgspecv1.MediaTypeImageLayerGzip,
					CompressionOperation: types.Compress,
					CompressionAlgorithm: &compression.Zstd,
				},
			},
			expectedFixture: "ociv1.zstd.manifest.json",
		},
		{
			name:          "zstd → gzip",
			sourceFixture: "ociv1.zstd.manifest.json",
			updates: []types.BlobInfo{
				{
					Digest:               "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f",
					Size:                 32654,
					MediaType:            imgspecv1.MediaTypeImageLayerZstd,
					CompressionOperation: types.Compress,
					CompressionAlgorithm: &compression.Gzip,
				},
				{
					Digest:               "sha256:3c3a4604a545cdc127456d94e421cd355bca5b528f4a9c1905b15da2eb4a4c6b",
					Size:                 16724,
					MediaType:            imgspecv1.MediaTypeImageLayerZstd,
					CompressionOperation: types.Compress,
					CompressionAlgorithm: &compression.Gzip,
				},
				{
					Digest:               "sha256:ec4b8955958665577945c89419d1af06b5f7636b4ac3da7f12184802ad867736",
					Size:                 73109,
					MediaType:            imgspecv1.MediaTypeImageLayerZstd,
					CompressionOperation: types.Compress,
					CompressionAlgorithm: &compression.Gzip,
				},
			},
			expectedFixture: "ociv1.manifest.json",
		},
		{
			name:          "zstd → uncompressed",
			sourceFixture: "ociv1.zstd.manifest.json",
			updates: []types.BlobInfo{
				{
					Digest:               "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f",
					Size:                 32654,
					MediaType:            imgspecv1.MediaTypeImageLayerZstd,
					CompressionOperation: types.Decompress,
				},
				{
					Digest:               "sha256:3c3a4604a545cdc127456d94e421cd355bca5b528f4a9c1905b15da2eb4a4c6b",
					Size:                 16724,
					MediaType:            imgspecv1.MediaTypeImageLayerZstd,
					CompressionOperation: types.Decompress,
				},
				{
					Digest:               "sha256:ec4b8955958665577945c89419d1af06b5f7636b4ac3da7f12184802ad867736",
					Size:                 73109,
					MediaType:            imgspecv1.MediaTypeImageLayerZstd,
					CompressionOperation: types.Decompress,
				},
			},
			expectedFixture: "ociv1.uncompressed.manifest.json",
		},
		{
			name:          "invalid compression operation",
			sourceFixture: "ociv1.zstd.manifest.json",
			updates: []types.BlobInfo{
				{
					Digest:               "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f",
					Size:                 32654,
					MediaType:            imgspecv1.MediaTypeImageLayerZstd,
					CompressionOperation: types.Compress,
					CompressionAlgorithm: &compression.Gzip,
				},
				{
					Digest:               "sha256:3c3a4604a545cdc127456d94e421cd355bca5b528f4a9c1905b15da2eb4a4c6b",
					Size:                 16724,
					MediaType:            imgspecv1.MediaTypeImageLayerZstd,
					CompressionOperation: 42, // MUST fail here
					CompressionAlgorithm: &compression.Gzip,
				},
				{
					Digest:               "sha256:ec4b8955958665577945c89419d1af06b5f7636b4ac3da7f12184802ad867736",
					Size:                 73109,
					MediaType:            imgspecv1.MediaTypeImageLayerZstd,
					CompressionOperation: types.Compress,
					CompressionAlgorithm: &compression.Gzip,
				},
			},
			expectedFixture: "",
		},
		{
			name:          "invalid compression algorithm",
			sourceFixture: "ociv1.zstd.manifest.json",
			updates: []types.BlobInfo{
				{
					Digest:               "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f",
					Size:                 32654,
					MediaType:            imgspecv1.MediaTypeImageLayerZstd,
					CompressionOperation: types.Compress,
					CompressionAlgorithm: &compression.Gzip,
				},
				{
					Digest:               "sha256:3c3a4604a545cdc127456d94e421cd355bca5b528f4a9c1905b15da2eb4a4c6b",
					Size:                 16724,
					MediaType:            imgspecv1.MediaTypeImageLayerZstd,
					CompressionOperation: 42,
					CompressionAlgorithm: &compression.Gzip,
				},
				{
					Digest:               "sha256:ec4b8955958665577945c89419d1af06b5f7636b4ac3da7f12184802ad867736",
					Size:                 73109,
					MediaType:            imgspecv1.MediaTypeImageLayerZstd,
					CompressionOperation: types.Compress,
					CompressionAlgorithm: &customCompression, // MUST fail here
				},
			},
			expectedFixture: "",
		},
		{
			name:          "gzip → uncompressed",
			sourceFixture: "ociv1.manifest.json",
			updates: []types.BlobInfo{
				{
					Digest:               "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f",
					Size:                 32654,
					MediaType:            imgspecv1.MediaTypeImageLayerGzip,
					CompressionOperation: types.Decompress,
				},
				{
					Digest:               "sha256:3c3a4604a545cdc127456d94e421cd355bca5b528f4a9c1905b15da2eb4a4c6b",
					Size:                 16724,
					MediaType:            imgspecv1.MediaTypeImageLayerGzip,
					CompressionOperation: types.Decompress,
				},
				{
					Digest:               "sha256:ec4b8955958665577945c89419d1af06b5f7636b4ac3da7f12184802ad867736",
					Size:                 73109,
					MediaType:            imgspecv1.MediaTypeImageLayerGzip,
					CompressionOperation: types.Decompress,
				},
			},
			expectedFixture: "ociv1.uncompressed.manifest.json",
		},
		{
			name:          "nondistributable → gzip",
			sourceFixture: "ociv1.nondistributable.manifest.json",
			updates: []types.BlobInfo{
				{
					Digest:               "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f",
					Size:                 32654,
					MediaType:            imgspecv1.MediaTypeImageLayerGzip,
					CompressionOperation: types.Compress,
					CompressionAlgorithm: &compression.Gzip,
				},
			},
			expectedFixture: "ociv1.nondistributable.gzip.manifest.json",
		},
		{
			name:          "nondistributable → zstd",
			sourceFixture: "ociv1.nondistributable.manifest.json",
			updates: []types.BlobInfo{
				{
					Digest:               "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f",
					Size:                 32654,
					MediaType:            imgspecv1.MediaTypeImageLayerGzip,
					CompressionOperation: types.Compress,
					CompressionAlgorithm: &compression.Zstd,
				},
			},
			expectedFixture: "ociv1.nondistributable.zstd.manifest.json",
		},
		{
			name:          "nondistributable gzip → uncompressed",
			sourceFixture: "ociv1.nondistributable.gzip.manifest.json",
			updates: []types.BlobInfo{
				{
					Digest:               "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f",
					Size:                 32654,
					MediaType:            imgspecv1.MediaTypeImageLayerGzip,
					CompressionOperation: types.Decompress,
				},
			},
			expectedFixture: "ociv1.nondistributable.manifest.json",
		},
	} {
		manifest := manifestOCI1FromFixture(t, c.sourceFixture)

		err := manifest.UpdateLayerInfos(c.updates)
		if c.expectedFixture == "" {
			assert.Error(t, err, c.name)
		} else {
			require.NoError(t, err, c.name)

			updatedManifestBytes, err := manifest.Serialize()
			require.NoError(t, err, c.name)

			expectedManifest := manifestOCI1FromFixture(t, c.expectedFixture)
			expectedManifestBytes, err := expectedManifest.Serialize()
			require.NoError(t, err, c.name)

			assert.Equal(t, string(expectedManifestBytes), string(updatedManifestBytes), c.name)
		}
	}
}

func TestOCI1Inspect(t *testing.T) {
	// Success is tested in image.TestManifestOCI1Inspect .
	m := manifestOCI1FromFixture(t, "ociv1.artifact.json")
	_, err := m.Inspect(func(info types.BlobInfo) ([]byte, error) {
		require.Equal(t, m.Config.Digest, info.Digest)
		// This just-enough-artifact contains a zero-byte config, sanity-check that’s till the case.
		require.Equal(t, int64(0), m.Config.Size)
		return []byte{}, nil
	})
	var expected NonImageArtifactError
	assert.ErrorAs(t, err, &expected)
}

func TestOCI1ImageID(t *testing.T) {
	m := manifestOCI1FromFixture(t, "ociv1.manifest.json")
	// These are not the real DiffID values, but they don’t actually matter in our implementation.
	id, err := m.ImageID([]digest.Digest{
		"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
	})
	require.NoError(t, err)
	assert.Equal(t, "b5b2b2c507a0944348e0303114d8d93aaaa081732b86451d9bce1f432a537bc7", id)

	m = manifestOCI1FromFixture(t, "ociv1.artifact.json")
	_, err = m.ImageID([]digest.Digest{})
	var expected NonImageArtifactError
	assert.ErrorAs(t, err, &expected)
}

func TestOCI1CanChangeLayerCompression(t *testing.T) {
	m := manifestOCI1FromFixture(t, "ociv1.manifest.json")

	assert.True(t, m.CanChangeLayerCompression(imgspecv1.MediaTypeImageLayerGzip))
	// Some projects like to use squashfs and other unspecified formats for layers; don’t touch those.
	assert.False(t, m.CanChangeLayerCompression("a completely unknown and quite possibly invalid MIME type"))

	artifact := manifestOCI1FromFixture(t, "ociv1.artifact.json")
	assert.False(t, artifact.CanChangeLayerCompression(imgspecv1.MediaTypeImageLayerGzip))
}

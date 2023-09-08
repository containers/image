package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/containers/image/v5/pkg/compression"
	"github.com/containers/image/v5/types"
	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func manifestSchema2FromFixture(t *testing.T, fixture string) *Schema2 {
	manifest, err := os.ReadFile(filepath.Join("fixtures", fixture))
	require.NoError(t, err)

	m, err := Schema2FromManifest(manifest)
	require.NoError(t, err)
	return m
}

func TestSupportedSchema2MediaType(t *testing.T) {
	type testData struct {
		m        string
		mustFail bool
	}
	data := []testData{
		{DockerV2Schema2MediaType, false},
		{DockerV2Schema2ConfigMediaType, false},
		{DockerV2Schema2LayerMediaType, false},
		{DockerV2SchemaLayerMediaTypeUncompressed, false},
		{DockerV2ListMediaType, false},
		{DockerV2Schema2ForeignLayerMediaType, false},
		{DockerV2Schema2ForeignLayerMediaTypeGzip, false},
		{"application/vnd.docker.image.rootfs.foreign.diff.unknown", true},
	}
	for _, d := range data {
		err := SupportedSchema2MediaType(d.m)
		if d.mustFail {
			assert.NotNil(t, err)
		} else {
			assert.Nil(t, err)
		}
	}
}

func TestSchema2FromManifest(t *testing.T) {
	validManifest, err := os.ReadFile(filepath.Join("fixtures", "v2s2.manifest.json"))
	require.NoError(t, err)

	parser := func(m []byte) error {
		_, err := Schema2FromManifest(m)
		return err
	}
	// Schema mismatch is rejected
	testManifestFixturesAreRejected(t, parser, []string{
		"schema2-to-schema1-by-docker.json",
		"v2list.manifest.json",
		"ociv1.manifest.json", "ociv1.image.index.json",
	})
	// Extra fields are rejected
	testValidManifestWithExtraFieldsIsRejected(t, parser, validManifest, []string{"fsLayers", "history", "manifests"})
}

func TestSchema2UpdateLayerInfos(t *testing.T) {
	for _, c := range []struct {
		name            string
		sourceFixture   string
		updates         []types.BlobInfo
		expectedFixture string // or "" to indicate an expected failure
	}{
		{
			name:          "gzip → zstd",
			sourceFixture: "v2s2.manifest.json",
			updates: []types.BlobInfo{
				{
					Digest:               "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f",
					Size:                 32654,
					MediaType:            DockerV2Schema2LayerMediaType,
					CompressionOperation: types.Compress,
					CompressionAlgorithm: &compression.Zstd,
				},
				{
					Digest:               "sha256:3c3a4604a545cdc127456d94e421cd355bca5b528f4a9c1905b15da2eb4a4c6b",
					Size:                 16724,
					MediaType:            DockerV2Schema2LayerMediaType,
					CompressionOperation: types.Compress,
					CompressionAlgorithm: &compression.Zstd,
				},
				{
					Digest:               "sha256:ec4b8955958665577945c89419d1af06b5f7636b4ac3da7f12184802ad867736",
					Size:                 73109,
					MediaType:            DockerV2Schema2LayerMediaType,
					CompressionOperation: types.Compress,
					CompressionAlgorithm: &compression.Zstd,
				},
			},
			expectedFixture: "", // zstd is not supported for docker images
		},
		{
			name:          "invalid compression operation",
			sourceFixture: "v2s2.manifest.json",
			updates: []types.BlobInfo{
				{
					Digest:               "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f",
					Size:                 32654,
					MediaType:            DockerV2Schema2LayerMediaType,
					CompressionOperation: types.Decompress,
				},
				{
					Digest:               "sha256:3c3a4604a545cdc127456d94e421cd355bca5b528f4a9c1905b15da2eb4a4c6b",
					Size:                 16724,
					MediaType:            DockerV2Schema2LayerMediaType,
					CompressionOperation: types.Decompress,
				},
				{
					Digest:               "sha256:ec4b8955958665577945c89419d1af06b5f7636b4ac3da7f12184802ad867736",
					Size:                 73109,
					MediaType:            DockerV2Schema2LayerMediaType,
					CompressionOperation: 42, // MUST fail here
				},
			},
			expectedFixture: "",
		},
		{
			name:          "invalid compression algorithm",
			sourceFixture: "v2s2.manifest.json",
			updates: []types.BlobInfo{
				{
					Digest:               "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f",
					Size:                 32654,
					MediaType:            DockerV2Schema2LayerMediaType,
					CompressionOperation: types.Compress,
					CompressionAlgorithm: &compression.Gzip,
				},
				{
					Digest:               "sha256:3c3a4604a545cdc127456d94e421cd355bca5b528f4a9c1905b15da2eb4a4c6b",
					Size:                 16724,
					MediaType:            DockerV2Schema2LayerMediaType,
					CompressionOperation: types.Compress,
					CompressionAlgorithm: &compression.Gzip,
				},
				{
					Digest:               "sha256:ec4b8955958665577945c89419d1af06b5f7636b4ac3da7f12184802ad867736",
					Size:                 73109,
					MediaType:            DockerV2Schema2LayerMediaType,
					CompressionOperation: types.Compress,
					CompressionAlgorithm: &compression.Zstd, // MUST fail here
				},
			},
			expectedFixture: "",
		},
		{
			name:          "nondistributable → gzip",
			sourceFixture: "v2s2.nondistributable.manifest.json",
			updates: []types.BlobInfo{
				{
					Digest:               "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f",
					Size:                 32654,
					MediaType:            DockerV2Schema2ForeignLayerMediaType,
					CompressionOperation: types.Compress,
					CompressionAlgorithm: &compression.Gzip,
				},
			},
			expectedFixture: "v2s2.nondistributable.gzip.manifest.json",
		},
		{
			name:          "nondistributable gzip → uncompressed",
			sourceFixture: "v2s2.nondistributable.gzip.manifest.json",
			updates: []types.BlobInfo{
				{
					Digest:               "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f",
					Size:                 32654,
					MediaType:            DockerV2Schema2ForeignLayerMediaType,
					CompressionOperation: types.Decompress,
				},
			},
			expectedFixture: "v2s2.nondistributable.manifest.json",
		},
		{
			name:          "uncompressed → gzip encrypted",
			sourceFixture: "v2s2.uncompressed.manifest.json",
			updates: []types.BlobInfo{
				{
					Digest:               "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					Size:                 32654,
					Annotations:          map[string]string{"org.opencontainers.image.enc.…": "layer1"},
					MediaType:            DockerV2SchemaLayerMediaTypeUncompressed,
					CompressionOperation: types.Compress,
					CompressionAlgorithm: &compression.Gzip,
					CryptoOperation:      types.Encrypt,
				},
				{
					Digest:               "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
					Size:                 16724,
					Annotations:          map[string]string{"org.opencontainers.image.enc.…": "layer2"},
					MediaType:            DockerV2SchemaLayerMediaTypeUncompressed,
					CompressionOperation: types.Compress,
					CompressionAlgorithm: &compression.Gzip,
					CryptoOperation:      types.Encrypt,
				},
				{
					Digest:               "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
					Size:                 73109,
					Annotations:          map[string]string{"org.opencontainers.image.enc.…": "layer2"},
					MediaType:            DockerV2SchemaLayerMediaTypeUncompressed,
					CompressionOperation: types.Compress,
					CompressionAlgorithm: &compression.Gzip,
					CryptoOperation:      types.Encrypt,
				},
			},
			expectedFixture: "", // Encryption is not supported
		},
		{
			name:          "gzip  → uncompressed decrypted", // We can’t represent encrypted images anyway, but verify that we reject decryption attempts.
			sourceFixture: "v2s2.manifest.json",
			updates: []types.BlobInfo{
				{
					Digest:               "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f",
					Size:                 32654,
					MediaType:            DockerV2Schema2LayerMediaType,
					CompressionOperation: types.Decompress,
					CryptoOperation:      types.Decrypt,
				},
				{
					Digest:               "sha256:3c3a4604a545cdc127456d94e421cd355bca5b528f4a9c1905b15da2eb4a4c6b",
					Size:                 16724,
					MediaType:            DockerV2Schema2LayerMediaType,
					CompressionOperation: types.Decompress,
					CryptoOperation:      types.Decrypt,
				},
				{
					Digest:               "sha256:ec4b8955958665577945c89419d1af06b5f7636b4ac3da7f12184802ad867736",
					Size:                 73109,
					MediaType:            DockerV2Schema2LayerMediaType,
					CompressionOperation: types.Decompress,
					CryptoOperation:      types.Decrypt,
				},
			},
			expectedFixture: "", // Decryption is not supported
		},
	} {
		manifest := manifestSchema2FromFixture(t, c.sourceFixture)

		err := manifest.UpdateLayerInfos(c.updates)
		if c.expectedFixture == "" {
			assert.Error(t, err, c.name)
		} else {
			require.NoError(t, err, c.name)

			updatedManifestBytes, err := manifest.Serialize()
			require.NoError(t, err, c.name)

			expectedManifest := manifestSchema2FromFixture(t, c.expectedFixture)
			expectedManifestBytes, err := expectedManifest.Serialize()
			require.NoError(t, err, c.name)

			assert.Equal(t, string(expectedManifestBytes), string(updatedManifestBytes), c.name)
		}
	}
}

func TestSchema2ImageID(t *testing.T) {
	m := manifestSchema2FromFixture(t, "v2s2.manifest.json")
	// These are not the real DiffID values, but they don’t actually matter in our implementation.
	id, err := m.ImageID([]digest.Digest{
		"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
	})
	require.NoError(t, err)
	assert.Equal(t, "b5b2b2c507a0944348e0303114d8d93aaaa081732b86451d9bce1f432a537bc7", id)
}

func TestSchema2CanChangeLayerCompression(t *testing.T) {
	m := manifestSchema2FromFixture(t, "v2s2.manifest.json")

	assert.True(t, m.CanChangeLayerCompression(DockerV2Schema2LayerMediaType))
	// Some projects like to use squashfs and other unspecified formats for layers; don’t touch those.
	assert.False(t, m.CanChangeLayerCompression("a completely unknown and quite possibly invalid MIME type"))
}

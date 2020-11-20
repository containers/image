package manifest

import (
	"testing"

	"github.com/containers/image/v5/pkg/compression"
	"github.com/containers/image/v5/types"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLayerInfosToStrings(t *testing.T) {
	strings := layerInfosToStrings([]LayerInfo{})
	assert.Equal(t, []string{}, strings)

	strings = layerInfosToStrings([]LayerInfo{
		{
			BlobInfo: types.BlobInfo{
				MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip",
				Digest:    "sha256:a3ed95caeb02ffe68cdd9fd84406680ae93d633cb16422d00e8a7c22955b46d4",
				Size:      32,
			},
			EmptyLayer: true,
		},
		{
			BlobInfo: types.BlobInfo{
				MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip",
				Digest:    "sha256:bbd6b22eb11afce63cc76f6bc41042d99f10d6024c96b655dafba930b8d25909",
				Size:      8841833,
			},
			EmptyLayer: false,
		},
		{
			BlobInfo: types.BlobInfo{
				MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip",
				Digest:    "sha256:960e52ecf8200cbd84e70eb2ad8678f4367e50d14357021872c10fa3fc5935fa",
				Size:      291,
			},
			EmptyLayer: false,
		},
		{
			BlobInfo: types.BlobInfo{
				MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip",
				Digest:    "sha256:a3ed95caeb02ffe68cdd9fd84406680ae93d633cb16422d00e8a7c22955b46d4",
				Size:      32,
			},
			EmptyLayer: true,
		},
	})
	assert.Equal(t, []string{
		"sha256:a3ed95caeb02ffe68cdd9fd84406680ae93d633cb16422d00e8a7c22955b46d4",
		"sha256:bbd6b22eb11afce63cc76f6bc41042d99f10d6024c96b655dafba930b8d25909",
		"sha256:960e52ecf8200cbd84e70eb2ad8678f4367e50d14357021872c10fa3fc5935fa",
		"sha256:a3ed95caeb02ffe68cdd9fd84406680ae93d633cb16422d00e8a7c22955b46d4",
	}, strings)
}

func TestCompressionVariantMIMEType(t *testing.T) {
	sets := []compressionMIMETypeSet{
		{mtsUncompressed: "AU", compression.Gzip.Name(): "AG" /* No zstd variant */},
		{mtsUncompressed: "BU", compression.Gzip.Name(): "BG", compression.Zstd.Name(): mtsUnsupportedMIMEType},
		{ /* No uncompressed variant */ compression.Gzip.Name(): "CG", compression.Zstd.Name(): "CZ"},
		{mtsUncompressed: "", compression.Gzip.Name(): "DG"},
	}

	for _, c := range []struct {
		input    string
		algo     *compression.Algorithm
		expected string
	}{
		{"AU", nil, "AU"}, {"AU", &compression.Gzip, "AG"}, {"AU", &compression.Zstd, ""},
		{"AG", nil, "AU"}, {"AG", &compression.Gzip, "AG"}, {"AG", &compression.Zstd, ""},
		{"BU", &compression.Zstd, ""},
		{"BG", &compression.Zstd, ""},
		{"CG", nil, ""}, {"CG", &compression.Zstd, "CZ"},
		{"CZ", nil, ""}, {"CZ", &compression.Gzip, "CG"},
		{"DG", nil, ""},
		{"unknown", nil, ""}, {"unknown", &compression.Gzip, ""},
		{"", nil, ""}, {"", &compression.Gzip, ""},
	} {
		res, err := compressionVariantMIMEType(sets, c.input, c.algo)
		if c.expected == "" {
			assert.Error(t, err, c.input)
		} else {
			require.NoError(t, err, c.input)
			assert.Equal(t, c.expected, res, c.input)
		}
	}
}

func TestUpdatedMIMEType(t *testing.T) {
	// all known types, PreserveOriginal
	preserve := []struct {
		compression []compressionMIMETypeSet
		mimeType    string
	}{
		{schema2CompressionMIMETypeSets, DockerV2Schema1MediaType},
		{schema2CompressionMIMETypeSets, DockerV2Schema1SignedMediaType},
		{schema2CompressionMIMETypeSets, DockerV2Schema2MediaType},
		{schema2CompressionMIMETypeSets, DockerV2Schema2ConfigMediaType},
		{schema2CompressionMIMETypeSets, DockerV2Schema2LayerMediaType},
		{schema2CompressionMIMETypeSets, DockerV2SchemaLayerMediaTypeUncompressed},
		{schema2CompressionMIMETypeSets, DockerV2ListMediaType},
		{schema2CompressionMIMETypeSets, DockerV2Schema2ForeignLayerMediaType},
		{schema2CompressionMIMETypeSets, DockerV2Schema2ForeignLayerMediaTypeGzip},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeDescriptor},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeLayoutHeader},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeImageManifest},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeImageIndex},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeImageLayer},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeImageLayerGzip},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeImageLayerZstd},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeImageLayerNonDistributable},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeImageLayerNonDistributableGzip},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeImageLayerNonDistributableZstd},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeImageConfig},
	}
	for i, c := range preserve {
		update := types.BlobInfo{
			MediaType:            c.mimeType,
			CompressionOperation: types.PreserveOriginal,
		}
		updatedType, err := updatedMIMEType(c.compression, c.mimeType, update)
		require.NoErrorf(t, err, "%d: updatedMIMEType(%q, %+v) failed unexpectedly", i, c.mimeType, update)
		assert.Equalf(t, c.mimeType, updatedType, "%d: updatedMIMEType(%q, %+v)", i, c.mimeType, update)
	}

	// known types where Decompress is expected to succeed
	decompressSuccess := []struct {
		compression []compressionMIMETypeSet
		mimeType    string
		updatedType string
	}{
		{schema2CompressionMIMETypeSets, DockerV2SchemaLayerMediaTypeUncompressed, DockerV2SchemaLayerMediaTypeUncompressed},
		{schema2CompressionMIMETypeSets, DockerV2Schema2ForeignLayerMediaTypeGzip, DockerV2Schema2ForeignLayerMediaType},
		{schema2CompressionMIMETypeSets, DockerV2Schema2LayerMediaType, DockerV2SchemaLayerMediaTypeUncompressed},
		{schema2CompressionMIMETypeSets, DockerV2Schema2ForeignLayerMediaType, DockerV2Schema2ForeignLayerMediaType},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeImageLayer, imgspecv1.MediaTypeImageLayer},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeImageLayerGzip, imgspecv1.MediaTypeImageLayer},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeImageLayerZstd, imgspecv1.MediaTypeImageLayer},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeImageLayerNonDistributable, imgspecv1.MediaTypeImageLayerNonDistributable},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeImageLayerNonDistributableGzip, imgspecv1.MediaTypeImageLayerNonDistributable},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeImageLayerNonDistributableZstd, imgspecv1.MediaTypeImageLayerNonDistributable},
	}
	for i, c := range decompressSuccess {
		update := types.BlobInfo{
			MediaType:            c.mimeType,
			CompressionOperation: types.Decompress,
		}
		updatedType, err := updatedMIMEType(c.compression, c.mimeType, update)
		require.NoErrorf(t, err, "%d: updatedMIMEType(%q, %+v) failed unexpectedly", i, c.mimeType, update)
		assert.Equalf(t, c.updatedType, updatedType, "%d: updatedMIMEType(%q, %+v)", i, c.mimeType, update)
	}

	// known types where Decompress is expected to fail
	decompressFailure := []struct {
		compression []compressionMIMETypeSet
		mimeType    string
	}{
		{schema2CompressionMIMETypeSets, DockerV2Schema1MediaType},
		{schema2CompressionMIMETypeSets, DockerV2Schema1SignedMediaType},
		{schema2CompressionMIMETypeSets, DockerV2Schema2MediaType},
		{schema2CompressionMIMETypeSets, DockerV2Schema2ConfigMediaType},
		{schema2CompressionMIMETypeSets, DockerV2ListMediaType},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeDescriptor},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeLayoutHeader},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeImageManifest},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeImageIndex},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeImageConfig},
	}
	for i, c := range decompressFailure {
		update := types.BlobInfo{
			MediaType:            c.mimeType,
			CompressionOperation: types.Decompress,
		}
		_, err := updatedMIMEType(c.compression, c.mimeType, update)
		require.Errorf(t, err, "%d: updatedMIMEType(%q, %+v) should have failed", i, c.mimeType, update)
	}

	require.Equalf(t, len(preserve), len(decompressSuccess)+len(decompressFailure), "missing some decompression tests")

	// all known types where Compress with gzip should succeed
	compressGzipSuccess := []struct {
		compression []compressionMIMETypeSet
		mimeType    string
		updatedType string
	}{
		{schema2CompressionMIMETypeSets, DockerV2Schema2LayerMediaType, DockerV2Schema2LayerMediaType},
		{schema2CompressionMIMETypeSets, DockerV2SchemaLayerMediaTypeUncompressed, DockerV2Schema2LayerMediaType},
		{schema2CompressionMIMETypeSets, DockerV2Schema2ForeignLayerMediaType, DockerV2Schema2ForeignLayerMediaTypeGzip},
		{schema2CompressionMIMETypeSets, DockerV2Schema2ForeignLayerMediaTypeGzip, DockerV2Schema2ForeignLayerMediaTypeGzip},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeImageLayer, imgspecv1.MediaTypeImageLayerGzip},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeImageLayerGzip, imgspecv1.MediaTypeImageLayerGzip},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeImageLayerZstd, imgspecv1.MediaTypeImageLayerGzip},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeImageLayerNonDistributable, imgspecv1.MediaTypeImageLayerNonDistributableGzip},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeImageLayerNonDistributableGzip, imgspecv1.MediaTypeImageLayerNonDistributableGzip},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeImageLayerNonDistributableZstd, imgspecv1.MediaTypeImageLayerNonDistributableGzip},
	}
	for i, c := range compressGzipSuccess {
		update := types.BlobInfo{
			MediaType:            c.mimeType,
			CompressionOperation: types.Compress,
			CompressionAlgorithm: &compression.Gzip,
		}
		updatedType, err := updatedMIMEType(c.compression, c.mimeType, update)
		require.NoErrorf(t, err, "%d: updatedMIMEType(%q, %+v) failed unexpectedly", i, c.mimeType, update)
		assert.Equalf(t, c.updatedType, updatedType, "%d: updatedMIMEType(%q, %+v)", i, c.mimeType, update)
	}

	// known types where Compress with gzip is expected to fail
	compressGzipFailure := []struct {
		compression []compressionMIMETypeSet
		mimeType    string
	}{
		{schema2CompressionMIMETypeSets, DockerV2Schema1MediaType},
		{schema2CompressionMIMETypeSets, DockerV2Schema1SignedMediaType},
		{schema2CompressionMIMETypeSets, DockerV2Schema2MediaType},
		{schema2CompressionMIMETypeSets, DockerV2Schema2ConfigMediaType},
		{schema2CompressionMIMETypeSets, DockerV2ListMediaType},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeDescriptor},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeLayoutHeader},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeImageManifest},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeImageIndex},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeImageConfig},
	}
	for i, c := range compressGzipFailure {
		update := types.BlobInfo{
			MediaType:            c.mimeType,
			CompressionOperation: types.Compress,
			CompressionAlgorithm: &compression.Gzip,
		}
		_, err := updatedMIMEType(c.compression, c.mimeType, update)
		require.Errorf(t, err, "%d: updatedMIMEType(%q, %+v) should have failed", i, c.mimeType, update)
	}

	require.Equalf(t, len(preserve), len(compressGzipSuccess)+len(compressGzipFailure), "missing some gzip compression tests")

	// known types where Compress with zstd is expected to succeed
	compressZstdSuccess := []struct {
		compression []compressionMIMETypeSet
		mimeType    string
		updatedType string
	}{
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeImageLayer, imgspecv1.MediaTypeImageLayerZstd},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeImageLayerGzip, imgspecv1.MediaTypeImageLayerZstd},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeImageLayerZstd, imgspecv1.MediaTypeImageLayerZstd},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeImageLayerNonDistributable, imgspecv1.MediaTypeImageLayerNonDistributableZstd},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeImageLayerNonDistributableGzip, imgspecv1.MediaTypeImageLayerNonDistributableZstd},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeImageLayerNonDistributableZstd, imgspecv1.MediaTypeImageLayerNonDistributableZstd},
	}
	for i, c := range compressZstdSuccess {
		update := types.BlobInfo{
			MediaType:            c.mimeType,
			CompressionOperation: types.Compress,
			CompressionAlgorithm: &compression.Zstd,
		}
		updatedType, err := updatedMIMEType(c.compression, c.mimeType, update)
		require.NoErrorf(t, err, "%d: updatedMIMEType(%q, %+v) failed unexpectedly", i, c.mimeType, update)
		assert.Equalf(t, c.updatedType, updatedType, "%d: updatedMIMEType(%q, %+v)", i, c.mimeType, update)
	}

	// known types where Compress with zstd is expected to fail
	compressZstdFailure := []struct {
		compression []compressionMIMETypeSet
		mimeType    string
	}{
		{schema2CompressionMIMETypeSets, DockerV2SchemaLayerMediaTypeUncompressed},
		{schema2CompressionMIMETypeSets, DockerV2Schema2LayerMediaType},
		{schema2CompressionMIMETypeSets, DockerV2Schema2ForeignLayerMediaType},
		{schema2CompressionMIMETypeSets, DockerV2Schema2ForeignLayerMediaTypeGzip},
		{schema2CompressionMIMETypeSets, DockerV2Schema1MediaType},
		{schema2CompressionMIMETypeSets, DockerV2Schema1SignedMediaType},
		{schema2CompressionMIMETypeSets, DockerV2Schema2MediaType},
		{schema2CompressionMIMETypeSets, DockerV2Schema2ConfigMediaType},
		{schema2CompressionMIMETypeSets, DockerV2ListMediaType},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeDescriptor},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeLayoutHeader},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeImageManifest},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeImageIndex},
		{oci1CompressionMIMETypeSets, imgspecv1.MediaTypeImageConfig},
	}
	for i, c := range compressZstdFailure {
		update := types.BlobInfo{
			MediaType:            c.mimeType,
			CompressionOperation: types.Compress,
			CompressionAlgorithm: &compression.Zstd,
		}
		_, err := updatedMIMEType(c.compression, c.mimeType, update)
		require.Errorf(t, err, "%d: updatedMIMEType(%q, %+v) should have failed", i, c.mimeType, update)
	}

	require.Equalf(t, len(preserve), len(compressZstdSuccess)+len(compressZstdFailure), "missing some zstd compression tests")
}

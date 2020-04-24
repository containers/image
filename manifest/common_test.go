package manifest

import (
	"testing"

	"github.com/containers/image/v5/pkg/compression"
	"github.com/containers/image/v5/types"
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

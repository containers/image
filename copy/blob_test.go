package copy

import (
	"fmt"
	"testing"

	"github.com/containers/image/v5/internal/private"
	"github.com/containers/image/v5/pkg/compression"
	"github.com/containers/image/v5/types"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
)

func TestUpdatedBlobInfoFromUpload(t *testing.T) {
	for _, c := range []struct {
		srcInfo  types.BlobInfo
		uploaded private.UploadedBlob
		expected types.BlobInfo
	}{
		{ // A straightforward upload with a known size
			srcInfo: types.BlobInfo{
				Digest:               "sha256:6a5a5368e0c2d3e5909184fa28ddfd56072e7ff3ee9a945876f7eee5896ef5bb",
				Size:                 51354364,
				URLs:                 []string{"https://layer.url"},
				Annotations:          map[string]string{"test-annotation-2": "two"},
				MediaType:            imgspecv1.MediaTypeImageLayerGzip,
				CompressionOperation: types.Compress,    // Might be set by blobCacheSource.LayerInfosForCopy
				CompressionAlgorithm: &compression.Gzip, // Set e.g. in copyLayer
				// CryptoOperation is not set by LayerInfos()
			},
			uploaded: private.UploadedBlob{
				Digest: "sha256:6a5a5368e0c2d3e5909184fa28ddfd56072e7ff3ee9a945876f7eee5896ef5bb",
				Size:   51354364,
			},
			expected: types.BlobInfo{
				Digest:               "sha256:6a5a5368e0c2d3e5909184fa28ddfd56072e7ff3ee9a945876f7eee5896ef5bb",
				Size:                 51354364,
				URLs:                 nil,
				Annotations:          map[string]string{"test-annotation-2": "two"},
				MediaType:            imgspecv1.MediaTypeImageLayerGzip,
				CompressionOperation: types.Compress,    // Might be set by blobCacheSource.LayerInfosForCopy
				CompressionAlgorithm: &compression.Gzip, // Set e.g. in copyLayer
				// CryptoOperation is set to the zero value
			},
		},
		{ // Upload determining the digest/size
			srcInfo: types.BlobInfo{
				Digest:               "",
				Size:                 -1,
				URLs:                 []string{"https://layer.url"},
				Annotations:          map[string]string{"test-annotation-2": "two"},
				MediaType:            imgspecv1.MediaTypeImageLayerGzip,
				CompressionOperation: types.Compress,    // Might be set by blobCacheSource.LayerInfosForCopy
				CompressionAlgorithm: &compression.Gzip, // Set e.g. in copyLayer
				// CryptoOperation is not set by LayerInfos()
			},
			uploaded: private.UploadedBlob{
				Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Size:   513543640,
			},
			expected: types.BlobInfo{
				Digest:               "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Size:                 513543640,
				URLs:                 nil,
				Annotations:          map[string]string{"test-annotation-2": "two"},
				MediaType:            imgspecv1.MediaTypeImageLayerGzip,
				CompressionOperation: types.Compress,    // Might be set by blobCacheSource.LayerInfosForCopy
				CompressionAlgorithm: &compression.Gzip, // Set e.g. in copyLayer
				// CryptoOperation is set to the zero value
			},
		},
	} {
		res := updatedBlobInfoFromUpload(c.srcInfo, c.uploaded)
		assert.Equal(t, c.expected, res, fmt.Sprintf("%#v", c.uploaded))
	}
}

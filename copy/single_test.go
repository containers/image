package copy

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/containers/image/v5/internal/private"
	"github.com/containers/image/v5/pkg/compression"
	compressiontypes "github.com/containers/image/v5/pkg/compression/types"
	"github.com/containers/image/v5/types"
	digest "github.com/opencontainers/go-digest"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdatedBlobInfoFromReuse(t *testing.T) {
	srcInfo := types.BlobInfo{
		Digest:               "sha256:6a5a5368e0c2d3e5909184fa28ddfd56072e7ff3ee9a945876f7eee5896ef5bb",
		Size:                 51354364,
		URLs:                 []string{"https://layer.url"},
		Annotations:          map[string]string{"test-annotation-2": "two"},
		MediaType:            imgspecv1.MediaTypeImageLayerGzip,
		CompressionOperation: types.Compress,    // Might be set by blobCacheSource.LayerInfosForCopy
		CompressionAlgorithm: &compression.Gzip, // Set e.g. in copyLayer
		// CryptoOperation is not set by LayerInfos()
	}

	for _, c := range []struct {
		reused   private.ReusedBlob
		expected types.BlobInfo
	}{
		{ // A straightforward reuse without substitution
			reused: private.ReusedBlob{
				Digest: "sha256:6a5a5368e0c2d3e5909184fa28ddfd56072e7ff3ee9a945876f7eee5896ef5bb",
				Size:   51354364,
				// CompressionOperation not set
				// CompressionAlgorithm not set
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
		{ // Reuse with substitution
			reused: private.ReusedBlob{
				Digest:                 "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Size:                   513543640,
				CompressionOperation:   types.Decompress,
				CompressionAlgorithm:   nil,
				CompressionAnnotations: map[string]string{"decompressed": "value"},
			},
			expected: types.BlobInfo{
				Digest:               "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Size:                 513543640,
				URLs:                 nil,
				Annotations:          map[string]string{"test-annotation-2": "two", "decompressed": "value"},
				MediaType:            imgspecv1.MediaTypeImageLayerGzip,
				CompressionOperation: types.Decompress,
				CompressionAlgorithm: nil,
				// CryptoOperation is set to the zero value
			},
		},
		{ // Reuse turning zstd into zstd:chunked
			reused: private.ReusedBlob{
				Digest:                 "sha256:6a5a5368e0c2d3e5909184fa28ddfd56072e7ff3ee9a945876f7eee5896ef5bb",
				Size:                   51354364,
				CompressionOperation:   types.Compress,
				CompressionAlgorithm:   &compression.ZstdChunked,
				CompressionAnnotations: map[string]string{"zstd-toc": "value"},
			},
			expected: types.BlobInfo{
				Digest:               "sha256:6a5a5368e0c2d3e5909184fa28ddfd56072e7ff3ee9a945876f7eee5896ef5bb",
				Size:                 51354364,
				URLs:                 nil,
				Annotations:          map[string]string{"test-annotation-2": "two", "zstd-toc": "value"},
				MediaType:            imgspecv1.MediaTypeImageLayerGzip,
				CompressionOperation: types.Compress,
				CompressionAlgorithm: &compression.ZstdChunked,
				// CryptoOperation is set to the zero value
			},
		},
	} {
		res := updatedBlobInfoFromReuse(srcInfo, c.reused)
		assert.Equal(t, c.expected, res, fmt.Sprintf("%#v", c.reused))
	}
}

func goDiffIDComputationGoroutineWithTimeout(layerStream io.ReadCloser, decompressor compressiontypes.DecompressorFunc) *diffIDResult {
	ch := make(chan diffIDResult)
	go diffIDComputationGoroutine(ch, layerStream, decompressor)
	timeout := time.After(time.Second)
	select {
	case res := <-ch:
		return &res
	case <-timeout:
		return nil
	}
}

func TestDiffIDComputationGoroutine(t *testing.T) {
	stream, err := os.Open("fixtures/Hello.uncompressed")
	require.NoError(t, err)
	res := goDiffIDComputationGoroutineWithTimeout(stream, nil)
	require.NotNil(t, res)
	assert.NoError(t, res.err)
	assert.Equal(t, "sha256:185f8db32271fe25f561a6fc938b2e264306ec304eda518007d1764826381969", res.digest.String())

	// Error reading input
	reader, writer := io.Pipe()
	err = writer.CloseWithError(errors.New("Expected error reading input in diffIDComputationGoroutine"))
	require.NoError(t, err)
	res = goDiffIDComputationGoroutineWithTimeout(reader, nil)
	require.NotNil(t, res)
	assert.Error(t, res.err)
}

func TestComputeDiffID(t *testing.T) {
	for _, c := range []struct {
		filename     string
		decompressor compressiontypes.DecompressorFunc
		result       digest.Digest
	}{
		{"fixtures/Hello.uncompressed", nil, "sha256:185f8db32271fe25f561a6fc938b2e264306ec304eda518007d1764826381969"},
		{"fixtures/Hello.gz", nil, "sha256:0bd4409dcd76476a263b8f3221b4ce04eb4686dec40bfdcc2e86a7403de13609"},
		{"fixtures/Hello.gz", compression.GzipDecompressor, "sha256:185f8db32271fe25f561a6fc938b2e264306ec304eda518007d1764826381969"},
		{"fixtures/Hello.zst", nil, "sha256:361a8e0372ad438a0316eb39a290318364c10b60d0a7e55b40aa3eafafc55238"},
		{"fixtures/Hello.zst", compression.ZstdDecompressor, "sha256:185f8db32271fe25f561a6fc938b2e264306ec304eda518007d1764826381969"},
	} {
		stream, err := os.Open(c.filename)
		require.NoError(t, err, c.filename)
		defer stream.Close()

		diffID, err := computeDiffID(stream, c.decompressor)
		require.NoError(t, err, c.filename)
		assert.Equal(t, c.result, diffID)
	}

	// Error initializing decompression
	_, err := computeDiffID(bytes.NewReader([]byte{}), compression.GzipDecompressor)
	assert.Error(t, err)

	// Error reading input
	reader, writer := io.Pipe()
	defer reader.Close()
	err = writer.CloseWithError(errors.New("Expected error reading input in computeDiffID"))
	require.NoError(t, err)
	_, err = computeDiffID(reader, nil)
	assert.Error(t, err)
}

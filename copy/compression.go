package copy

import (
	"io"

	"github.com/containers/image/v5/pkg/compression"
	compressiontypes "github.com/containers/image/v5/pkg/compression/types"
	"github.com/containers/image/v5/types"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// bpDetectCompressionStepData contains data that the copy pipeline needs about the “detect compression” step.
type bpDetectCompressionStepData struct {
	isCompressed bool
	format       compressiontypes.Algorithm        // Valid if isCompressed
	decompressor compressiontypes.DecompressorFunc // Valid if isCompressed
}

// blobPipelineDetectCompressionStep updates *stream to detect its current compression format.
// srcInfo is only used for error messages.
// Returns data for other steps.
func blobPipelineDetectCompressionStep(stream *sourceStream, srcInfo types.BlobInfo) (bpDetectCompressionStepData, error) {
	// This requires us to “peek ahead” into the stream to read the initial part, which requires us to chain through another io.Reader returned by DetectCompression.
	format, decompressor, reader, err := compression.DetectCompressionFormat(stream.reader) // We could skip this in some cases, but let's keep the code path uniform
	if err != nil {
		return bpDetectCompressionStepData{}, errors.Wrapf(err, "reading blob %s", srcInfo.Digest)
	}
	stream.reader = reader

	res := bpDetectCompressionStepData{
		isCompressed: decompressor != nil,
		format:       format,
		decompressor: decompressor,
	}
	if expectedFormat, known := expectedCompressionFormats[stream.info.MediaType]; known && res.isCompressed && format.Name() != expectedFormat.Name() {
		logrus.Debugf("blob %s with type %s should be compressed with %s, but compressor appears to be %s", srcInfo.Digest.String(), srcInfo.MediaType, expectedFormat.Name(), format.Name())
	}
	return res, nil
}

// doCompression reads all input from src and writes its compressed equivalent to dest.
func doCompression(dest io.Writer, src io.Reader, metadata map[string]string, compressionFormat compressiontypes.Algorithm, compressionLevel *int) error {
	compressor, err := compression.CompressStreamWithMetadata(dest, metadata, compressionFormat, compressionLevel)
	if err != nil {
		return err
	}

	buf := make([]byte, compressionBufferSize)

	_, err = io.CopyBuffer(compressor, src, buf) // Sets err to nil, i.e. causes dest.Close()
	if err != nil {
		compressor.Close()
		return err
	}

	return compressor.Close()
}

// compressGoroutine reads all input from src and writes its compressed equivalent to dest.
func (c *copier) compressGoroutine(dest *io.PipeWriter, src io.Reader, metadata map[string]string, compressionFormat compressiontypes.Algorithm) {
	err := errors.New("Internal error: unexpected panic in compressGoroutine")
	defer func() { // Note that this is not the same as {defer dest.CloseWithError(err)}; we need err to be evaluated lazily.
		_ = dest.CloseWithError(err) // CloseWithError(nil) is equivalent to Close(), always returns nil
	}()

	err = doCompression(dest, src, metadata, compressionFormat, c.compressionLevel)
}

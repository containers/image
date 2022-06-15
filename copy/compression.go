package copy

import (
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

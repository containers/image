package copy

import (
	"io"

	internalblobinfocache "github.com/containers/image/v5/internal/blobinfocache"
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

// bpCompressionStepData contains data that the copy pipeline needs about the compression step.
type bpCompressionStepData struct {
	operation              types.LayerCompression      // Operation to use for updating the blob metadata.
	uploadedAlgorithm      *compressiontypes.Algorithm // An algorithm parameter for the compressionOperation edits.
	compressionMetadata    map[string]string           // Annotations that should be set on the uploaded blob. WARNING: This is only set after the srcStream.reader is fully consumed.
	srcCompressorName      string                      // Compressor name to record in the blob info cache for the source blob.
	uploadedCompressorName string                      // Compressor name to record in the blob info cache for the uploaded blob.
	closers                []io.Closer                 // Objects to close after the upload is done, if any.
}

// blobPipelineCompressionStep updates *stream to compress and/or decompress it.
// srcInfo is only used for error messages.
// Returns data for other steps; the caller should eventually call updateCompressionEdits and perhaps recordValidatedBlobData,
// and must eventually call close.
func (c *copier) blobPipelineCompressionStep(stream *sourceStream, canModifyBlob bool,
	detected bpDetectCompressionStepData) (*bpCompressionStepData, error) {
	// WARNING: If you are adding new reasons to change the blob, update also the OptimizeDestinationImageAlreadyExists
	// short-circuit conditions
	compressionMetadata := map[string]string{}
	var operation types.LayerCompression
	var uploadedAlgorithm *compressiontypes.Algorithm
	srcCompressorName := internalblobinfocache.Uncompressed
	if detected.isCompressed {
		srcCompressorName = detected.format.Name()
	}
	var uploadedCompressorName string
	var closers []io.Closer
	succeeded := false
	defer func() {
		if !succeeded {
			for _, c := range closers {
				c.Close()
			}
		}
	}()
	if canModifyBlob && isOciEncrypted(stream.info.MediaType) {
		// PreserveOriginal due to any compression not being able to be done on an encrypted blob unless decrypted
		logrus.Debugf("Using original blob without modification for encrypted blob")
		operation = types.PreserveOriginal
		srcCompressorName = internalblobinfocache.UnknownCompression
		uploadedAlgorithm = nil
		uploadedCompressorName = internalblobinfocache.UnknownCompression
	} else if canModifyBlob && c.dest.DesiredLayerCompression() == types.Compress && !detected.isCompressed {
		logrus.Debugf("Compressing blob on the fly")
		operation = types.Compress
		pipeReader, pipeWriter := io.Pipe()
		closers = append(closers, pipeReader)

		if c.compressionFormat != nil {
			uploadedAlgorithm = c.compressionFormat
		} else {
			uploadedAlgorithm = defaultCompressionFormat
		}
		// If this fails while writing data, it will do pipeWriter.CloseWithError(); if it fails otherwise,
		// e.g. because we have exited and due to pipeReader.Close() above further writing to the pipe has failed,
		// we don’t care.
		go c.compressGoroutine(pipeWriter, stream.reader, compressionMetadata, *uploadedAlgorithm) // Closes pipeWriter
		stream.reader = pipeReader
		stream.info = types.BlobInfo{ // FIXME? Should we preserve more data in src.info?
			Digest: "",
			Size:   -1,
		}
		uploadedCompressorName = uploadedAlgorithm.Name()
	} else if canModifyBlob && c.dest.DesiredLayerCompression() == types.Compress && detected.isCompressed &&
		c.compressionFormat != nil && c.compressionFormat.Name() != detected.format.Name() {
		// When the blob is compressed, but the desired format is different, it first needs to be decompressed and finally
		// re-compressed using the desired format.
		logrus.Debugf("Blob will be converted")

		operation = types.PreserveOriginal
		s, err := detected.decompressor(stream.reader)
		if err != nil {
			return nil, err
		}
		closers = append(closers, s)

		pipeReader, pipeWriter := io.Pipe()
		closers = append(closers, pipeReader)

		uploadedAlgorithm = c.compressionFormat
		go c.compressGoroutine(pipeWriter, s, compressionMetadata, *uploadedAlgorithm) // Closes pipeWriter

		stream.reader = pipeReader
		stream.info = types.BlobInfo{ // FIXME? Should we preserve more data in src.info?
			Digest: "",
			Size:   -1,
		}
		uploadedCompressorName = uploadedAlgorithm.Name()
	} else if canModifyBlob && c.dest.DesiredLayerCompression() == types.Decompress && detected.isCompressed {
		logrus.Debugf("Blob will be decompressed")
		operation = types.Decompress
		s, err := detected.decompressor(stream.reader)
		if err != nil {
			return nil, err
		}
		closers = append(closers, s)
		stream.reader = s
		stream.info = types.BlobInfo{ // FIXME? Should we preserve more data in src.info?
			Digest: "",
			Size:   -1,
		}
		uploadedAlgorithm = nil
		uploadedCompressorName = internalblobinfocache.Uncompressed
	} else {
		// PreserveOriginal might also need to recompress the original blob if the desired compression format is different.
		logrus.Debugf("Using original blob without modification")
		operation = types.PreserveOriginal
		// Remember if the original blob was compressed, and if so how, so that if
		// LayerInfosForCopy() returned something that differs from what was in the
		// source's manifest, and UpdatedImage() needs to call UpdateLayerInfos(),
		// it will be able to correctly derive the MediaType for the copied blob.
		if detected.isCompressed {
			uploadedAlgorithm = &detected.format
		} else {
			uploadedAlgorithm = nil
		}
		uploadedCompressorName = srcCompressorName
	}
	succeeded = true
	return &bpCompressionStepData{
		operation:              operation,
		uploadedAlgorithm:      uploadedAlgorithm,
		compressionMetadata:    compressionMetadata,
		srcCompressorName:      srcCompressorName,
		uploadedCompressorName: uploadedCompressorName,
		closers:                closers,
	}, nil
}

// updateCompressionEdits sets *operation, *algorithm and updates *annotations, if necessary.
func (d *bpCompressionStepData) updateCompressionEdits(operation *types.LayerCompression, algorithm **compressiontypes.Algorithm, annotations *map[string]string) {
	*operation = d.operation
	// If we can modify the layer's blob, set the desired algorithm for it to be set in the manifest.
	*algorithm = d.uploadedAlgorithm
	if *annotations == nil {
		*annotations = map[string]string{}
	}
	for k, v := range d.compressionMetadata {
		(*annotations)[k] = v
	}
}

// recordValidatedBlobData updates b.blobInfoCache with data about the created uploadedInfo adnd the original srcInfo.
// This must ONLY be called if all data has been validated by OUR code, and is not comming from third parties.
func (d *bpCompressionStepData) recordValidatedDigestData(c *copier, uploadedInfo types.BlobInfo, srcInfo types.BlobInfo,
	encryptionStep *bpEncryptionStepData, decryptionStep *bpDecryptionStepData) error {
	// Don’t record any associations that involve encrypted data. This is a bit crude,
	// some blob substitutions (replacing pulls of encrypted data with local reuse of known decryption outcomes)
	// might be safe, but it’s not trivially obvious, so let’s be conservative for now.
	// This crude approach also means we don’t need to record whether a blob is encrypted
	// in the blob info cache (which would probably be necessary for any more complex logic),
	// and the simplicity is attractive.
	if !encryptionStep.encrypting && !decryptionStep.decrypting {
		// If d.operation != types.PreserveOriginal, we now have two reliable digest values:
		// srcinfo.Digest describes the pre-d.operation input, verified by digestingReader
		// uploadedInfo.Digest describes the post-d.operation output, computed by PutBlob
		// (because stream.info.Digest == "", this must have been computed afresh).
		switch d.operation {
		case types.PreserveOriginal:
			break // Do nothing, we have only one digest and we might not have even verified it.
		case types.Compress:
			c.blobInfoCache.RecordDigestUncompressedPair(uploadedInfo.Digest, srcInfo.Digest)
		case types.Decompress:
			c.blobInfoCache.RecordDigestUncompressedPair(srcInfo.Digest, uploadedInfo.Digest)
		default:
			return errors.Errorf("Internal error: Unexpected d.operation value %#v", d.operation)
		}
	}
	if d.uploadedCompressorName != "" && d.uploadedCompressorName != internalblobinfocache.UnknownCompression {
		c.blobInfoCache.RecordDigestCompressorName(uploadedInfo.Digest, d.uploadedCompressorName)
	}
	if srcInfo.Digest != "" && d.srcCompressorName != "" && d.srcCompressorName != internalblobinfocache.UnknownCompression {
		c.blobInfoCache.RecordDigestCompressorName(srcInfo.Digest, d.srcCompressorName)
	}
	return nil
}

// close closes objects that carry state throughout the compression/decompression operation.
func (d *bpCompressionStepData) close() {
	for _, c := range d.closers {
		c.Close()
	}
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

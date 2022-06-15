package copy

import (
	"context"
	"io"
	"strings"

	internalblobinfocache "github.com/containers/image/v5/internal/blobinfocache"
	"github.com/containers/image/v5/internal/private"
	"github.com/containers/image/v5/pkg/compression"
	compressiontypes "github.com/containers/image/v5/pkg/compression/types"
	"github.com/containers/image/v5/types"
	"github.com/containers/ocicrypt"
	digest "github.com/opencontainers/go-digest"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// copyBlobFromStream copies a blob with srcInfo (with known Digest and Annotations and possibly known Size) from srcReader to dest,
// perhaps sending a copy to an io.Writer if getOriginalLayerCopyWriter != nil,
// perhaps (de/re/)compressing it if canModifyBlob,
// and returns a complete blobInfo of the copied blob.
func (c *copier) copyBlobFromStream(ctx context.Context, srcReader io.Reader, srcInfo types.BlobInfo,
	getOriginalLayerCopyWriter func(decompressor compressiontypes.DecompressorFunc) io.Writer,
	canModifyBlob bool, isConfig bool, toEncrypt bool, bar *progressBar, layerIndex int, emptyLayer bool) (types.BlobInfo, error) {
	if isConfig { // This is guaranteed by the caller, but set it here to be explicit.
		canModifyBlob = false
	}

	// The copying happens through a pipeline of connected io.Readers;
	// that pipeline is built by updating stream.
	// === Input: srcReader
	stream := sourceStream{
		reader: srcReader,
		info:   srcInfo,
	}

	// === Process input through digestingReader to validate against the expected digest.
	// Be paranoid; in case PutBlob somehow managed to ignore an error from digestingReader,
	// use a separate validation failure indicator.
	// Note that for this check we don't use the stronger "validationSucceeded" indicator, because
	// dest.PutBlob may detect that the layer already exists, in which case we don't
	// read stream to the end, and validation does not happen.
	digestingReader, err := newDigestingReader(stream.reader, srcInfo.Digest)
	if err != nil {
		return types.BlobInfo{}, errors.Wrapf(err, "preparing to verify blob %s", srcInfo.Digest)
	}
	stream.reader = digestingReader

	// === Update progress bars
	stream.reader = bar.ProxyReader(stream.reader)

	// === Decrypt the stream, if required.
	var decrypted bool
	if isOciEncrypted(stream.info.MediaType) && c.ociDecryptConfig != nil {
		newDesc := imgspecv1.Descriptor{
			Annotations: stream.info.Annotations,
		}

		var d digest.Digest
		stream.reader, d, err = ocicrypt.DecryptLayer(c.ociDecryptConfig, stream.reader, newDesc, false)
		if err != nil {
			return types.BlobInfo{}, errors.Wrapf(err, "decrypting layer %s", srcInfo.Digest)
		}

		stream.info.Digest = d
		stream.info.Size = -1
		for k := range stream.info.Annotations {
			if strings.HasPrefix(k, "org.opencontainers.image.enc") {
				delete(stream.info.Annotations, k)
			}
		}
		decrypted = true
	}

	// === Detect compression of the input stream.
	// This requires us to “peek ahead” into the stream to read the initial part, which requires us to chain through another io.Reader returned by DetectCompression.
	compressionFormat, decompressor, detectCompressionFormatReader, err := compression.DetectCompressionFormat(stream.reader) // We could skip this in some cases, but let's keep the code path uniform
	if err != nil {
		return types.BlobInfo{}, errors.Wrapf(err, "reading blob %s", srcInfo.Digest)
	}
	stream.reader = detectCompressionFormatReader
	isCompressed := decompressor != nil
	if expectedCompressionFormat, known := expectedCompressionFormats[stream.info.MediaType]; known && isCompressed && compressionFormat.Name() != expectedCompressionFormat.Name() {
		logrus.Debugf("blob %s with type %s should be compressed with %s, but compressor appears to be %s", srcInfo.Digest.String(), srcInfo.MediaType, expectedCompressionFormat.Name(), compressionFormat.Name())
	}

	// === Send a copy of the original, uncompressed, stream, to a separate path if necessary.
	var originalLayerReader io.Reader // DO NOT USE this other than to drain the input if no other consumer in the pipeline has done so.
	if getOriginalLayerCopyWriter != nil {
		stream.reader = io.TeeReader(stream.reader, getOriginalLayerCopyWriter(decompressor))
		originalLayerReader = stream.reader
	}

	compressionMetadata := map[string]string{}
	// === Deal with layer compression/decompression if necessary
	// WARNING: If you are adding new reasons to change the blob, update also the OptimizeDestinationImageAlreadyExists
	// short-circuit conditions
	var compressionOperation types.LayerCompression
	var uploadCompressionFormat *compressiontypes.Algorithm
	srcCompressorName := internalblobinfocache.Uncompressed
	if isCompressed {
		srcCompressorName = compressionFormat.Name()
	}
	var uploadCompressorName string
	if canModifyBlob && isOciEncrypted(stream.info.MediaType) {
		// PreserveOriginal due to any compression not being able to be done on an encrypted blob unless decrypted
		logrus.Debugf("Using original blob without modification for encrypted blob")
		compressionOperation = types.PreserveOriginal
		srcCompressorName = internalblobinfocache.UnknownCompression
		uploadCompressionFormat = nil
		uploadCompressorName = internalblobinfocache.UnknownCompression
	} else if canModifyBlob && c.dest.DesiredLayerCompression() == types.Compress && !isCompressed {
		logrus.Debugf("Compressing blob on the fly")
		compressionOperation = types.Compress
		pipeReader, pipeWriter := io.Pipe()
		defer pipeReader.Close()

		if c.compressionFormat != nil {
			uploadCompressionFormat = c.compressionFormat
		} else {
			uploadCompressionFormat = defaultCompressionFormat
		}
		// If this fails while writing data, it will do pipeWriter.CloseWithError(); if it fails otherwise,
		// e.g. because we have exited and due to pipeReader.Close() above further writing to the pipe has failed,
		// we don’t care.
		go c.compressGoroutine(pipeWriter, stream.reader, compressionMetadata, *uploadCompressionFormat) // Closes pipeWriter
		stream.reader = pipeReader
		stream.info = types.BlobInfo{ // FIXME? Should we preserve more data in src.info?
			Digest: "",
			Size:   -1,
		}
		uploadCompressorName = uploadCompressionFormat.Name()
	} else if canModifyBlob && c.dest.DesiredLayerCompression() == types.Compress && isCompressed &&
		c.compressionFormat != nil && c.compressionFormat.Name() != compressionFormat.Name() {
		// When the blob is compressed, but the desired format is different, it first needs to be decompressed and finally
		// re-compressed using the desired format.
		logrus.Debugf("Blob will be converted")

		compressionOperation = types.PreserveOriginal
		s, err := decompressor(stream.reader)
		if err != nil {
			return types.BlobInfo{}, err
		}
		defer s.Close()

		pipeReader, pipeWriter := io.Pipe()
		defer pipeReader.Close()

		uploadCompressionFormat = c.compressionFormat
		go c.compressGoroutine(pipeWriter, s, compressionMetadata, *uploadCompressionFormat) // Closes pipeWriter

		stream.reader = pipeReader
		stream.info = types.BlobInfo{ // FIXME? Should we preserve more data in src.info?
			Digest: "",
			Size:   -1,
		}
		uploadCompressorName = uploadCompressionFormat.Name()
	} else if canModifyBlob && c.dest.DesiredLayerCompression() == types.Decompress && isCompressed {
		logrus.Debugf("Blob will be decompressed")
		compressionOperation = types.Decompress
		s, err := decompressor(stream.reader)
		if err != nil {
			return types.BlobInfo{}, err
		}
		defer s.Close()
		stream.reader = s
		stream.info = types.BlobInfo{ // FIXME? Should we preserve more data in src.info?
			Digest: "",
			Size:   -1,
		}
		uploadCompressionFormat = nil
		uploadCompressorName = internalblobinfocache.Uncompressed
	} else {
		// PreserveOriginal might also need to recompress the original blob if the desired compression format is different.
		logrus.Debugf("Using original blob without modification")
		compressionOperation = types.PreserveOriginal
		// Remember if the original blob was compressed, and if so how, so that if
		// LayerInfosForCopy() returned something that differs from what was in the
		// source's manifest, and UpdatedImage() needs to call UpdateLayerInfos(),
		// it will be able to correctly derive the MediaType for the copied blob.
		if isCompressed {
			uploadCompressionFormat = &compressionFormat
		} else {
			uploadCompressionFormat = nil
		}
		uploadCompressorName = srcCompressorName
	}

	// === Encrypt the stream for valid mediatypes if ociEncryptConfig provided
	var (
		encrypted bool
		finalizer ocicrypt.EncryptLayerFinalizer
	)
	if toEncrypt {
		if decrypted {
			return types.BlobInfo{}, errors.New("Unable to support both decryption and encryption in the same copy")
		}

		if !isOciEncrypted(srcInfo.MediaType) && c.ociEncryptConfig != nil {
			var annotations map[string]string
			if !decrypted {
				annotations = srcInfo.Annotations
			}
			desc := imgspecv1.Descriptor{
				MediaType:   srcInfo.MediaType,
				Digest:      srcInfo.Digest,
				Size:        srcInfo.Size,
				Annotations: annotations,
			}

			s, fin, err := ocicrypt.EncryptLayer(c.ociEncryptConfig, stream.reader, desc)
			if err != nil {
				return types.BlobInfo{}, errors.Wrapf(err, "encrypting blob %s", srcInfo.Digest)
			}

			finalizer = fin
			stream.reader = s
			stream.info.Digest = ""
			stream.info.Size = -1
			encrypted = true
		}
	}

	// === Report progress using the c.progress channel, if required.
	if c.progress != nil && c.progressInterval > 0 {
		progressReader := newProgressReader(
			stream.reader,
			c.progress,
			c.progressInterval,
			srcInfo,
		)
		defer progressReader.reportDone()
		stream.reader = progressReader
	}

	// === Finally, send the layer stream to dest.
	options := private.PutBlobOptions{
		Cache:      c.blobInfoCache,
		IsConfig:   isConfig,
		EmptyLayer: emptyLayer,
	}
	if !isConfig {
		options.LayerIndex = &layerIndex
	}
	uploadedInfo, err := c.dest.PutBlobWithOptions(ctx, &errorAnnotationReader{stream.reader}, stream.info, options)
	if err != nil {
		return types.BlobInfo{}, errors.Wrap(err, "writing blob")
	}

	uploadedInfo.Annotations = stream.info.Annotations

	uploadedInfo.CompressionOperation = compressionOperation
	// If we can modify the layer's blob, set the desired algorithm for it to be set in the manifest.
	uploadedInfo.CompressionAlgorithm = uploadCompressionFormat
	if decrypted {
		uploadedInfo.CryptoOperation = types.Decrypt
	} else if encrypted {
		encryptAnnotations, err := finalizer()
		if err != nil {
			return types.BlobInfo{}, errors.Wrap(err, "Unable to finalize encryption")
		}
		uploadedInfo.CryptoOperation = types.Encrypt
		if uploadedInfo.Annotations == nil {
			uploadedInfo.Annotations = map[string]string{}
		}
		for k, v := range encryptAnnotations {
			uploadedInfo.Annotations[k] = v
		}
	}

	// This is fairly horrible: the writer from getOriginalLayerCopyWriter wants to consume
	// all of the input (to compute DiffIDs), even if dest.PutBlob does not need it.
	// So, read everything from originalLayerReader, which will cause the rest to be
	// sent there if we are not already at EOF.
	if getOriginalLayerCopyWriter != nil {
		logrus.Debugf("Consuming rest of the original blob to satisfy getOriginalLayerCopyWriter")
		_, err := io.Copy(io.Discard, originalLayerReader)
		if err != nil {
			return types.BlobInfo{}, errors.Wrapf(err, "reading input blob %s", srcInfo.Digest)
		}
	}

	if digestingReader.validationFailed { // Coverage: This should never happen.
		return types.BlobInfo{}, errors.Errorf("Internal error writing blob %s, digest verification failed but was ignored", srcInfo.Digest)
	}
	if stream.info.Digest != "" && uploadedInfo.Digest != stream.info.Digest {
		return types.BlobInfo{}, errors.Errorf("Internal error writing blob %s, blob with digest %s saved with digest %s", srcInfo.Digest, stream.info.Digest, uploadedInfo.Digest)
	}
	if digestingReader.validationSucceeded {
		// Don’t record any associations that involve encrypted data. This is a bit crude,
		// some blob substitutions (replacing pulls of encrypted data with local reuse of known decryption outcomes)
		// might be safe, but it’s not trivially obvious, so let’s be conservative for now.
		// This crude approach also means we don’t need to record whether a blob is encrypted
		// in the blob info cache (which would probably be necessary for any more complex logic),
		// and the simplicity is attractive.
		if !encrypted && !decrypted {
			// If compressionOperation != types.PreserveOriginal, we now have two reliable digest values:
			// srcinfo.Digest describes the pre-compressionOperation input, verified by digestingReader
			// uploadedInfo.Digest describes the post-compressionOperation output, computed by PutBlob
			// (because stream.info.Digest == "", this must have been computed afresh).
			switch compressionOperation {
			case types.PreserveOriginal:
				break // Do nothing, we have only one digest and we might not have even verified it.
			case types.Compress:
				c.blobInfoCache.RecordDigestUncompressedPair(uploadedInfo.Digest, srcInfo.Digest)
			case types.Decompress:
				c.blobInfoCache.RecordDigestUncompressedPair(srcInfo.Digest, uploadedInfo.Digest)
			default:
				return types.BlobInfo{}, errors.Errorf("Internal error: Unexpected compressionOperation value %#v", compressionOperation)
			}
		}
		if uploadCompressorName != "" && uploadCompressorName != internalblobinfocache.UnknownCompression {
			c.blobInfoCache.RecordDigestCompressorName(uploadedInfo.Digest, uploadCompressorName)
		}
		if srcInfo.Digest != "" && srcCompressorName != "" && srcCompressorName != internalblobinfocache.UnknownCompression {
			c.blobInfoCache.RecordDigestCompressorName(srcInfo.Digest, srcCompressorName)
		}
	}

	// Copy all the metadata generated by the compressor into the annotations.
	if uploadedInfo.Annotations == nil {
		uploadedInfo.Annotations = map[string]string{}
	}
	for k, v := range compressionMetadata {
		uploadedInfo.Annotations[k] = v
	}

	return uploadedInfo, nil
}

// sourceStream encapsulates an input consumed by copyBlobFromStream, in progress of being built.
// This allows handles of individual aspects to build the copy pipeline without _too much_
// specific cooperation by the caller.
//
// We are currently very far from a generalized plug-and-play API for building/consuming the pipeline
// without specific knowledge of various aspects in copyBlobFromStream; that may come one day.
type sourceStream struct {
	reader io.Reader
	info   types.BlobInfo // corresponding to the data available in reader.
}

// errorAnnotationReader wraps the io.Reader passed to PutBlob for annotating the error happened during read.
// These errors are reported as PutBlob errors, so we would otherwise misleadingly attribute them to the copy destination.
type errorAnnotationReader struct {
	reader io.Reader
}

// Read annotates the error happened during read
func (r errorAnnotationReader) Read(b []byte) (n int, err error) {
	n, err = r.reader.Read(b)
	if err != io.EOF {
		return n, errors.Wrapf(err, "happened during read")
	}
	return n, err
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
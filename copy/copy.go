package copy

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"reflect"
	"strings"

	pb "gopkg.in/cheggaaa/pb.v1"

	"github.com/Sirupsen/logrus"
	"github.com/containers/image/image"
	"github.com/containers/image/signature"
	"github.com/containers/image/transports"
	"github.com/containers/image/types"
)

// supportedDigests lists the supported blob digest types.
var supportedDigests = map[string]func() hash.Hash{
	"sha256": sha256.New,
}

type digestingReader struct {
	source           io.Reader
	digest           hash.Hash
	expectedDigest   []byte
	validationFailed bool
}

// newDigestingReader returns an io.Reader implementation with contents of source, which will eventually return a non-EOF error
// and set validationFailed to true if the source stream does not match expectedDigestString.
func newDigestingReader(source io.Reader, expectedDigestString string) (*digestingReader, error) {
	fields := strings.SplitN(expectedDigestString, ":", 2)
	if len(fields) != 2 {
		return nil, fmt.Errorf("Invalid digest specification %s", expectedDigestString)
	}
	fn, ok := supportedDigests[fields[0]]
	if !ok {
		return nil, fmt.Errorf("Invalid digest specification %s: unknown digest type %s", expectedDigestString, fields[0])
	}
	digest := fn()
	expectedDigest, err := hex.DecodeString(fields[1])
	if err != nil {
		return nil, fmt.Errorf("Invalid digest value %s: %v", expectedDigestString, err)
	}
	if len(expectedDigest) != digest.Size() {
		return nil, fmt.Errorf("Invalid digest specification %s: length %d does not match %d", expectedDigestString, len(expectedDigest), digest.Size())
	}
	return &digestingReader{
		source:           source,
		digest:           digest,
		expectedDigest:   expectedDigest,
		validationFailed: false,
	}, nil
}

func (d *digestingReader) Read(p []byte) (int, error) {
	n, err := d.source.Read(p)
	if n > 0 {
		if n2, err := d.digest.Write(p[:n]); n2 != n || err != nil {
			// Coverage: This should not happen, the hash.Hash interface requires
			// d.digest.Write to never return an error, and the io.Writer interface
			// requires n2 == len(input) if no error is returned.
			return 0, fmt.Errorf("Error updating digest during verification: %d vs. %d, %v", n2, n, err)
		}
	}
	if err == io.EOF {
		actualDigest := d.digest.Sum(nil)
		if subtle.ConstantTimeCompare(actualDigest, d.expectedDigest) != 1 {
			d.validationFailed = true
			return 0, fmt.Errorf("Digest did not match, expected %s, got %s", hex.EncodeToString(d.expectedDigest), hex.EncodeToString(actualDigest))
		}
	}
	return n, err
}

// Options allows supplying non-default configuration modifying the behavior of CopyImage.
type Options struct {
	RemoveSignatures bool   // Remove any pre-existing signatures. SignBy will still add a new signature.
	SignBy           string // If non-empty, asks for a signature to be added during the copy, and specifies a key ID, as accepted by signature.NewGPGSigningMechanism().SignDockerManifest(),
	ReportWriter     io.Writer
}

// Image copies image from srcRef to destRef, using policyContext to validate source image admissibility.
func Image(ctx *types.SystemContext, policyContext *signature.PolicyContext, destRef, srcRef types.ImageReference, options *Options) error {
	if options.ReportWriter == nil {
		options.ReportWriter = ioutil.Discard
	}
	writeReport := func(f string, a ...interface{}) {
		fmt.Fprintf(options.ReportWriter, f, a...)
	}
	dest, err := destRef.NewImageDestination(ctx)
	if err != nil {
		return fmt.Errorf("Error initializing destination %s: %v", transports.ImageName(destRef), err)
	}
	defer dest.Close()

	rawSource, err := srcRef.NewImageSource(ctx, dest.SupportedManifestMIMETypes())
	if err != nil {
		return fmt.Errorf("Error initializing source %s: %v", transports.ImageName(srcRef), err)
	}
	src := image.FromSource(rawSource)
	defer src.Close()

	// Please keep this policy check BEFORE reading any other information about the image.
	if allowed, err := policyContext.IsRunningImageAllowed(src); !allowed || err != nil { // Be paranoid and fail if either return value indicates so.
		return fmt.Errorf("Source image rejected: %v", err)
	}

	writeReport("Getting image source manifest\n")
	manifest, _, err := src.Manifest()
	if err != nil {
		return fmt.Errorf("Error reading manifest: %v", err)
	}

	var sigs [][]byte
	if options != nil && options.RemoveSignatures {
		sigs = [][]byte{}
	} else {
		writeReport("Getting image source signatures\n")
		s, err := src.Signatures()
		if err != nil {
			return fmt.Errorf("Error reading signatures: %v", err)
		}
		sigs = s
	}
	if len(sigs) != 0 {
		writeReport("Checking if image destination supports signatures\n")
		if err := dest.SupportsSignatures(); err != nil {
			return fmt.Errorf("Can not copy signatures: %v", err)
		}
	}
	canModifyManifest := len(sigs) == 0

	writeReport("Getting image source configuration\n")
	srcConfigInfo, err := src.ConfigInfo()
	if err != nil {
		return fmt.Errorf("Error parsing manifest: %v", err)
	}
	if srcConfigInfo.Digest != "" {
		writeReport("Uploading blob %s\n", srcConfigInfo.Digest)
		destConfigInfo, err := copyBlob(dest, rawSource, srcConfigInfo, false, options.ReportWriter)
		if err != nil {
			return err
		}
		if destConfigInfo.Digest != srcConfigInfo.Digest {
			return fmt.Errorf("Internal error: copying uncompressed config blob %s changed digest to %s", srcConfigInfo.Digest, destConfigInfo.Digest)
		}
	}

	srcLayerInfos, err := src.LayerInfos()
	if err != nil {
		return fmt.Errorf("Error parsing manifest: %v", err)
	}
	destLayerInfos := []types.BlobInfo{}
	copiedLayers := map[string]types.BlobInfo{}
	for _, srcLayer := range srcLayerInfos {
		destLayer, ok := copiedLayers[srcLayer.Digest]
		if !ok {
			writeReport("Uploading blob %s\n", srcLayer.Digest)
			destLayer, err = copyBlob(dest, rawSource, srcLayer, canModifyManifest, options.ReportWriter)
			if err != nil {
				return err
			}
			copiedLayers[srcLayer.Digest] = destLayer
		}
		destLayerInfos = append(destLayerInfos, destLayer)
	}

	manifestUpdates := types.ManifestUpdateOptions{}
	if layerDigestsDiffer(srcLayerInfos, destLayerInfos) {
		manifestUpdates.LayerInfos = destLayerInfos
	}

	if !reflect.DeepEqual(manifestUpdates, types.ManifestUpdateOptions{}) {
		if !canModifyManifest {
			return fmt.Errorf("Internal error: copy needs an updated manifest but that was known to be forbidden")
		}
		manifest, err = src.UpdatedManifest(manifestUpdates)
		if err != nil {
			return fmt.Errorf("Error creating an updated manifest: %v", err)
		}
	}

	if options != nil && options.SignBy != "" {
		mech, err := signature.NewGPGSigningMechanism()
		if err != nil {
			return fmt.Errorf("Error initializing GPG: %v", err)
		}
		dockerReference := dest.Reference().DockerReference()
		if dockerReference == nil {
			return fmt.Errorf("Cannot determine canonical Docker reference for destination %s", transports.ImageName(dest.Reference()))
		}

		writeReport("Signing manifest\n")
		newSig, err := signature.SignDockerManifest(manifest, dockerReference.String(), mech, options.SignBy)
		if err != nil {
			return fmt.Errorf("Error creating signature: %v", err)
		}
		sigs = append(sigs, newSig)
	}

	writeReport("Uploading manifest to image destination\n")
	if err := dest.PutManifest(manifest); err != nil {
		return fmt.Errorf("Error writing manifest: %v", err)
	}

	writeReport("Storing signatures\n")
	if err := dest.PutSignatures(sigs); err != nil {
		return fmt.Errorf("Error writing signatures: %v", err)
	}

	if err := dest.Commit(); err != nil {
		return fmt.Errorf("Error committing the finished image: %v", err)
	}

	return nil
}

// layerDigestsDiffer return true iff the digests in a and b differ (ignoring sizes and possible other fields)
func layerDigestsDiffer(a, b []types.BlobInfo) bool {
	if len(a) != len(b) {
		return true
	}
	for i := range a {
		if a[i].Digest != b[i].Digest {
			return true
		}
	}
	return false
}

// copyBlob copies a blob with srcInfo (with known Digest and possibly known Size) in src to dest, perhaps compressing it if canCompress,
// and returns a complete blobInfo of the copied blob.
func copyBlob(dest types.ImageDestination, src types.ImageSource, srcInfo types.BlobInfo, canCompress bool, reportWriter io.Writer) (types.BlobInfo, error) {
	srcStream, srcBlobSize, err := src.GetBlob(srcInfo.Digest) // We currently completely ignore srcInfo.Size throughout.
	if err != nil {
		return types.BlobInfo{}, fmt.Errorf("Error reading blob %s: %v", srcInfo.Digest, err)
	}
	defer srcStream.Close()

	// Be paranoid; in case PutBlob somehow managed to ignore an error from digestingReader,
	// use a separate validation failure indicator.
	// Note that we don't use a stronger "validationSucceeded" indicator, because
	// dest.PutBlob may detect that the layer already exists, in which case we don't
	// read stream to the end, and validation does not happen.
	digestingReader, err := newDigestingReader(srcStream, srcInfo.Digest)
	if err != nil {
		return types.BlobInfo{}, fmt.Errorf("Error preparing to verify blob %s: %v", srcInfo.Digest, err)
	}
	var destStream io.Reader = digestingReader
	isCompressed, destStream, err := isStreamCompressed(destStream) // We could skip this in some cases, but let's keep the code path uniform
	if err != nil {
		return types.BlobInfo{}, fmt.Errorf("Error reading blob %s: %v", srcInfo.Digest, err)
	}

	bar := pb.New(int(srcInfo.Size)).SetUnits(pb.U_BYTES)
	bar.Output = reportWriter
	bar.SetMaxWidth(80)
	bar.ShowTimeLeft = false
	bar.ShowPercent = false
	bar.Start()
	destStream = bar.NewProxyReader(destStream)

	defer fmt.Fprint(reportWriter, "\n")

	var inputInfo types.BlobInfo
	if !canCompress || isCompressed || !dest.ShouldCompressLayers() {
		logrus.Debugf("Using original blob without modification")
		inputInfo.Digest = srcInfo.Digest
		inputInfo.Size = srcBlobSize
	} else {
		logrus.Debugf("Compressing blob on the fly")
		pipeReader, pipeWriter := io.Pipe()
		defer pipeReader.Close()

		// If this fails while writing data, it will do pipeWriter.CloseWithError(); if it fails otherwise,
		// e.g. because we have exited and due to pipeReader.Close() above further writing to the pipe has failed,
		// we don’t care.
		go compressGoroutine(pipeWriter, destStream) // Closes pipeWriter
		destStream = pipeReader
		inputInfo.Digest = ""
		inputInfo.Size = -1
	}

	uploadedInfo, err := dest.PutBlob(destStream, inputInfo)
	if err != nil {
		return types.BlobInfo{}, fmt.Errorf("Error writing blob: %v", err)
	}
	if digestingReader.validationFailed { // Coverage: This should never happen.
		return types.BlobInfo{}, fmt.Errorf("Internal error uploading blob %s, digest verification failed but was ignored", srcInfo.Digest)
	}
	if inputInfo.Digest != "" && uploadedInfo.Digest != inputInfo.Digest {
		return types.BlobInfo{}, fmt.Errorf("Internal error uploading blob %s, blob with digest %s uploaded with digest %s", srcInfo.Digest, inputInfo.Digest, uploadedInfo.Digest)
	}
	return uploadedInfo, nil
}

// compressionPrefixes is an internal implementation detail of isStreamCompressed
var compressionPrefixes = map[string][]byte{
	"gzip":  {0x1F, 0x8B, 0x08},                   // gzip (RFC 1952)
	"bzip2": {0x42, 0x5A, 0x68},                   // bzip2 (decompress.c:BZ2_decompress)
	"xz":    {0xFD, 0x37, 0x7A, 0x58, 0x5A, 0x00}, // xz (/usr/share/doc/xz/xz-file-format.txt)
}

// isStreamCompressed returns true if input is recognized as a compressed format.
// Because it consumes the start of input, other consumers must use the returned io.Reader instead to also read from the beginning.
func isStreamCompressed(input io.Reader) (bool, io.Reader, error) {
	buffer := [8]byte{}

	n, err := io.ReadAtLeast(input, buffer[:], len(buffer))
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		// This is a “real” error. We could just ignore it this time, process the data we have, and hope that the source will report the same error again.
		// Instead, fail immediately with the original error cause instead of a possibly secondary/misleading error returned later.
		return false, nil, err
	}

	isCompressed := false
	for algo, prefix := range compressionPrefixes {
		if bytes.HasPrefix(buffer[:n], prefix) {
			logrus.Debugf("Detected compression format %s", algo)
			isCompressed = true
			break
		}
	}
	if !isCompressed {
		logrus.Debugf("No compression detected")
	}

	return isCompressed, io.MultiReader(bytes.NewReader(buffer[:n]), input), nil
}

// compressGoroutine reads all input from src and writes its compressed equivalent to dest.
func compressGoroutine(dest *io.PipeWriter, src io.Reader) {
	err := errors.New("Internal error: unexpected panic in compressGoroutine")
	defer func() { // Note that this is not the same as {defer dest.CloseWithError(err)}; we need err to be evaluated lazily.
		dest.CloseWithError(err) // CloseWithError(nil) is equivalent to Close()
	}()

	zipper := gzip.NewWriter(dest)
	defer zipper.Close()

	_, err = io.Copy(zipper, src) // Sets err to nil, i.e. causes dest.Close()
}

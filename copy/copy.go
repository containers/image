package copy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/containers/image/v5/docker/reference"
	internalblobinfocache "github.com/containers/image/v5/internal/blobinfocache"
	"github.com/containers/image/v5/internal/image"
	"github.com/containers/image/v5/internal/imagedestination"
	"github.com/containers/image/v5/internal/imagesource"
	internalManifest "github.com/containers/image/v5/internal/manifest"
	"github.com/containers/image/v5/internal/private"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/pkg/blobinfocache"
	"github.com/containers/image/v5/pkg/compression"
	compressiontypes "github.com/containers/image/v5/pkg/compression/types"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/signature/signer"
	"github.com/containers/image/v5/transports"
	"github.com/containers/image/v5/types"
	encconfig "github.com/containers/ocicrypt/config"
	digest "github.com/opencontainers/go-digest"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
	"golang.org/x/sync/semaphore"
	"golang.org/x/term"
)

var (
	// ErrDecryptParamsMissing is returned if there is missing decryption parameters
	ErrDecryptParamsMissing = errors.New("Necessary DecryptParameters not present")

	// maxParallelDownloads is used to limit the maximum number of parallel
	// downloads.  Let's follow Firefox by limiting it to 6.
	maxParallelDownloads = uint(6)

	// defaultCompressionFormat is used if the destination transport requests
	// compression, and the user does not explicitly instruct us to use an algorithm.
	defaultCompressionFormat = &compression.Gzip
)

// compressionBufferSize is the buffer size used to compress a blob
var compressionBufferSize = 1048576

// expectedCompressionFormats is used to check if a blob with a specified media type is compressed
// using the algorithm that the media type says it should be compressed with
var expectedCompressionFormats = map[string]*compressiontypes.Algorithm{
	imgspecv1.MediaTypeImageLayerGzip:      &compression.Gzip,
	imgspecv1.MediaTypeImageLayerZstd:      &compression.Zstd,
	manifest.DockerV2Schema2LayerMediaType: &compression.Gzip,
}

// copier allows us to keep track of diffID values for blobs, and other
// data shared across one or more images in a possible manifest list.
// The owner must call close() when done.
type copier struct {
	dest                          private.ImageDestination
	rawSource                     private.ImageSource
	reportWriter                  io.Writer
	progressOutput                io.Writer
	progressInterval              time.Duration
	progress                      chan types.ProgressProperties
	blobInfoCache                 internalblobinfocache.BlobInfoCache2
	compressionFormat             *compressiontypes.Algorithm // Compression algorithm to use, if the user explicitly requested one, or nil.
	compressionLevel              *int
	ociDecryptConfig              *encconfig.DecryptConfig
	ociEncryptConfig              *encconfig.EncryptConfig
	concurrentBlobCopiesSemaphore *semaphore.Weighted // Limits the amount of concurrently copied blobs
	downloadForeignLayers         bool
	signers                       []*signer.Signer // Signers to use to create new signatures for the image
	signersToClose                []*signer.Signer // Signers that should be closed when this copier is destroyed.
}

const (
	// CopySystemImage is the default value which, when set in
	// Options.ImageListSelection, indicates that the caller expects only one
	// image to be copied, so if the source reference refers to a list of
	// images, one that matches the current system will be selected.
	CopySystemImage ImageListSelection = iota
	// CopyAllImages is a value which, when set in Options.ImageListSelection,
	// indicates that the caller expects to copy multiple images, and if
	// the source reference refers to a list, that the list and every image
	// to which it refers will be copied.  If the source reference refers
	// to a list, the target reference can not accept lists, an error
	// should be returned.
	CopyAllImages
	// CopySpecificImages is a value which, when set in
	// Options.ImageListSelection, indicates that the caller expects the
	// source reference to be either a single image or a list of images,
	// and if the source reference is a list, wants only specific instances
	// from it copied (or none of them, if the list of instances to copy is
	// empty), along with the list itself.  If the target reference can
	// only accept one image (i.e., it cannot accept lists), an error
	// should be returned.
	CopySpecificImages
)

// ImageListSelection is one of CopySystemImage, CopyAllImages, or
// CopySpecificImages, to control whether, when the source reference is a list,
// copy.Image() copies only an image which matches the current runtime
// environment, or all images which match the supplied reference, or only
// specific images from the source reference.
type ImageListSelection int

// Options allows supplying non-default configuration modifying the behavior of CopyImage.
type Options struct {
	RemoveSignatures bool // Remove any pre-existing signatures. Signers and SignBy… will still add a new signature.
	// Signers to use to add signatures during the copy.
	// Callers are still responsible for closing these Signer objects; they can be reused for multiple copy.Image operations in a row.
	Signers                          []*signer.Signer
	SignBy                           string          // If non-empty, asks for a signature to be added during the copy, and specifies a key ID, as accepted by signature.NewGPGSigningMechanism().SignDockerManifest(),
	SignPassphrase                   string          // Passphrase to use when signing with the key ID from `SignBy`.
	SignBySigstorePrivateKeyFile     string          // If non-empty, asks for a signature to be added during the copy, using a sigstore private key file at the provided path.
	SignSigstorePrivateKeyPassphrase []byte          // Passphrase to use when signing with `SignBySigstorePrivateKeyFile`.
	SignIdentity                     reference.Named // Identify to use when signing, defaults to the docker reference of the destination

	ReportWriter     io.Writer
	SourceCtx        *types.SystemContext
	DestinationCtx   *types.SystemContext
	ProgressInterval time.Duration                 // time to wait between reports to signal the progress channel
	Progress         chan types.ProgressProperties // Reported to when ProgressInterval has arrived for a single artifact+offset.

	// Preserve digests, and fail if we cannot.
	PreserveDigests bool
	// manifest MIME type of image set by user. "" is default and means use the autodetection to the manifest MIME type
	ForceManifestMIMEType string
	ImageListSelection    ImageListSelection // set to either CopySystemImage (the default), CopyAllImages, or CopySpecificImages to control which instances we copy when the source reference is a list; ignored if the source reference is not a list
	Instances             []digest.Digest    // if ImageListSelection is CopySpecificImages, copy only these instances and the list itself
	// Give priority to pulling gzip images if multiple images are present when configured to OptionalBoolTrue,
	// prefers the best compression if this is configured as OptionalBoolFalse. Choose automatically (and the choice may change over time)
	// if this is set to OptionalBoolUndefined (which is the default behavior, and recommended for most callers).
	// This only affects CopySystemImage.
	PreferGzipInstances types.OptionalBool

	// If OciEncryptConfig is non-nil, it indicates that an image should be encrypted.
	// The encryption options is derived from the construction of EncryptConfig object.
	// Note: During initial encryption process of a layer, the resultant digest is not known
	// during creation, so newDigestingReader has to be set with validateDigest = false
	OciEncryptConfig *encconfig.EncryptConfig
	// OciEncryptLayers represents the list of layers to encrypt.
	// If nil, don't encrypt any layers.
	// If non-nil and len==0, denotes encrypt all layers.
	// integers in the slice represent 0-indexed layer indices, with support for negative
	// indexing. i.e. 0 is the first layer, -1 is the last (top-most) layer.
	OciEncryptLayers *[]int
	// OciDecryptConfig contains the config that can be used to decrypt an image if it is
	// encrypted if non-nil. If nil, it does not attempt to decrypt an image.
	OciDecryptConfig *encconfig.DecryptConfig

	// A weighted semaphore to limit the amount of concurrently copied layers and configs. Applies to all copy operations using the semaphore. If set, MaxParallelDownloads is ignored.
	ConcurrentBlobCopiesSemaphore *semaphore.Weighted

	// MaxParallelDownloads indicates the maximum layers to pull at the same time. Applies to a single copy operation. A reasonable default is used if this is left as 0. Ignored if ConcurrentBlobCopiesSemaphore is set.
	MaxParallelDownloads uint

	// When OptimizeDestinationImageAlreadyExists is set, optimize the copy assuming that the destination image already
	// exists (and is equivalent). Making the eventual (no-op) copy more performant for this case. Enabling the option
	// is slightly pessimistic if the destination image doesn't exist, or is not equivalent.
	OptimizeDestinationImageAlreadyExists bool

	// Download layer contents with "nondistributable" media types ("foreign" layers) and translate the layer media type
	// to not indicate "nondistributable".
	DownloadForeignLayers bool
}

// validateImageListSelection returns an error if the passed-in value is not one that we recognize as a valid ImageListSelection value
func validateImageListSelection(selection ImageListSelection) error {
	switch selection {
	case CopySystemImage, CopyAllImages, CopySpecificImages:
		return nil
	default:
		return fmt.Errorf("Invalid value for options.ImageListSelection: %d", selection)
	}
}

// Image copies image from srcRef to destRef, using policyContext to validate
// source image admissibility.  It returns the manifest which was written to
// the new copy of the image.
func Image(ctx context.Context, policyContext *signature.PolicyContext, destRef, srcRef types.ImageReference, options *Options) (copiedManifest []byte, retErr error) {
	// NOTE this function uses an output parameter for the error return value.
	// Setting this and returning is the ideal way to return an error.
	//
	// the defers in this routine will wrap the error return with its own errors
	// which can be valuable context in the middle of a multi-streamed copy.
	if options == nil {
		options = &Options{}
	}

	if err := validateImageListSelection(options.ImageListSelection); err != nil {
		return nil, err
	}

	reportWriter := io.Discard

	if options.ReportWriter != nil {
		reportWriter = options.ReportWriter
	}

	publicDest, err := destRef.NewImageDestination(ctx, options.DestinationCtx)
	if err != nil {
		return nil, fmt.Errorf("initializing destination %s: %w", transports.ImageName(destRef), err)
	}
	dest := imagedestination.FromPublic(publicDest)
	defer func() {
		if err := dest.Close(); err != nil {
			if retErr != nil {
				retErr = fmt.Errorf(" (dest: %v): %w", err, retErr)
			} else {
				retErr = fmt.Errorf(" (dest: %v)", err)
			}
		}
	}()

	publicRawSource, err := srcRef.NewImageSource(ctx, options.SourceCtx)
	if err != nil {
		return nil, fmt.Errorf("initializing source %s: %w", transports.ImageName(srcRef), err)
	}
	rawSource := imagesource.FromPublic(publicRawSource)
	defer func() {
		if err := rawSource.Close(); err != nil {
			if retErr != nil {
				retErr = fmt.Errorf(" (src: %v): %w", err, retErr)
			} else {
				retErr = fmt.Errorf(" (src: %v)", err)
			}
		}
	}()

	// If reportWriter is not a TTY (e.g., when piping to a file), do not
	// print the progress bars to avoid long and hard to parse output.
	// Instead use printCopyInfo() to print single line "Copying ..." messages.
	progressOutput := reportWriter
	if !isTTY(reportWriter) {
		progressOutput = io.Discard
	}

	c := &copier{
		dest:             dest,
		rawSource:        rawSource,
		reportWriter:     reportWriter,
		progressOutput:   progressOutput,
		progressInterval: options.ProgressInterval,
		progress:         options.Progress,
		// FIXME? The cache is used for sources and destinations equally, but we only have a SourceCtx and DestinationCtx.
		// For now, use DestinationCtx (because blob reuse changes the behavior of the destination side more); eventually
		// we might want to add a separate CommonCtx — or would that be too confusing?
		blobInfoCache:         internalblobinfocache.FromBlobInfoCache(blobinfocache.DefaultCache(options.DestinationCtx)),
		ociDecryptConfig:      options.OciDecryptConfig,
		ociEncryptConfig:      options.OciEncryptConfig,
		downloadForeignLayers: options.DownloadForeignLayers,
	}
	defer c.close()

	// Set the concurrentBlobCopiesSemaphore if we can copy layers in parallel.
	if dest.HasThreadSafePutBlob() && rawSource.HasThreadSafeGetBlob() {
		c.concurrentBlobCopiesSemaphore = options.ConcurrentBlobCopiesSemaphore
		if c.concurrentBlobCopiesSemaphore == nil {
			max := options.MaxParallelDownloads
			if max == 0 {
				max = maxParallelDownloads
			}
			c.concurrentBlobCopiesSemaphore = semaphore.NewWeighted(int64(max))
		}
	} else {
		c.concurrentBlobCopiesSemaphore = semaphore.NewWeighted(int64(1))
		if options.ConcurrentBlobCopiesSemaphore != nil {
			if err := options.ConcurrentBlobCopiesSemaphore.Acquire(ctx, 1); err != nil {
				return nil, fmt.Errorf("acquiring semaphore for concurrent blob copies: %w", err)
			}
			defer options.ConcurrentBlobCopiesSemaphore.Release(1)
		}
	}

	if options.DestinationCtx != nil {
		// Note that compressionFormat and compressionLevel can be nil.
		c.compressionFormat = options.DestinationCtx.CompressionFormat
		c.compressionLevel = options.DestinationCtx.CompressionLevel
	}

	if err := c.setupSigners(options); err != nil {
		return nil, err
	}

	unparsedToplevel := image.UnparsedInstance(rawSource, nil)
	multiImage, err := isMultiImage(ctx, unparsedToplevel)
	if err != nil {
		return nil, fmt.Errorf("determining manifest MIME type for %s: %w", transports.ImageName(srcRef), err)
	}

	if !multiImage {
		// The simple case: just copy a single image.
		if copiedManifest, _, _, err = c.copyOneImage(ctx, policyContext, options, unparsedToplevel, unparsedToplevel, nil); err != nil {
			return nil, err
		}
	} else if options.ImageListSelection == CopySystemImage {
		// This is a manifest list, and we weren't asked to copy multiple images.  Choose a single image that
		// matches the current system to copy, and copy it.
		mfest, manifestType, err := unparsedToplevel.Manifest(ctx)
		if err != nil {
			return nil, fmt.Errorf("reading manifest for %s: %w", transports.ImageName(srcRef), err)
		}
		manifestList, err := internalManifest.ListFromBlob(mfest, manifestType)
		if err != nil {
			return nil, fmt.Errorf("parsing primary manifest as list for %s: %w", transports.ImageName(srcRef), err)
		}
		instanceDigest, err := manifestList.ChooseInstanceByCompression(options.SourceCtx, options.PreferGzipInstances) // try to pick one that matches options.SourceCtx
		if err != nil {
			return nil, fmt.Errorf("choosing an image from manifest list %s: %w", transports.ImageName(srcRef), err)
		}
		logrus.Debugf("Source is a manifest list; copying (only) instance %s for current system", instanceDigest)
		unparsedInstance := image.UnparsedInstance(rawSource, &instanceDigest)

		if copiedManifest, _, _, err = c.copyOneImage(ctx, policyContext, options, unparsedToplevel, unparsedInstance, nil); err != nil {
			return nil, fmt.Errorf("copying system image from manifest list: %w", err)
		}
	} else { /* options.ImageListSelection == CopyAllImages or options.ImageListSelection == CopySpecificImages, */
		// If we were asked to copy multiple images and can't, that's an error.
		if !supportsMultipleImages(c.dest) {
			return nil, fmt.Errorf("copying multiple images: destination transport %q does not support copying multiple images as a group", destRef.Transport().Name())
		}
		// Copy some or all of the images.
		switch options.ImageListSelection {
		case CopyAllImages:
			logrus.Debugf("Source is a manifest list; copying all instances")
		case CopySpecificImages:
			logrus.Debugf("Source is a manifest list; copying some instances")
		}
		if copiedManifest, err = c.copyMultipleImages(ctx, policyContext, options, unparsedToplevel); err != nil {
			return nil, err
		}
	}

	if err := c.dest.Commit(ctx, unparsedToplevel); err != nil {
		return nil, fmt.Errorf("committing the finished image: %w", err)
	}

	return copiedManifest, nil
}

// close tears down state owned by copier.
func (c *copier) close() {
	for i, s := range c.signersToClose {
		if err := s.Close(); err != nil {
			logrus.Warnf("Error closing per-copy signer %d: %v", i+1, err)
		}
	}
}

// Checks if the destination supports accepting multiple images by checking if it can support
// manifest types that are lists of other manifests.
func supportsMultipleImages(dest types.ImageDestination) bool {
	mtypes := dest.SupportedManifestMIMETypes()
	if len(mtypes) == 0 {
		// Anything goes!
		return true
	}
	return slices.ContainsFunc(mtypes, manifest.MIMETypeIsMultiImage)
}

// copyMultipleImages copies some or all of an image list's instances, using
// policyContext to validate source image admissibility.
func (c *copier) copyMultipleImages(ctx context.Context, policyContext *signature.PolicyContext, options *Options, unparsedToplevel *image.UnparsedImage) (copiedManifest []byte, retErr error) {
	// Parse the list and get a copy of the original value after it's re-encoded.
	manifestList, manifestType, err := unparsedToplevel.Manifest(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading manifest list: %w", err)
	}
	originalList, err := internalManifest.ListFromBlob(manifestList, manifestType)
	if err != nil {
		return nil, fmt.Errorf("parsing manifest list %q: %w", string(manifestList), err)
	}
	updatedList := originalList.CloneInternal()

	sigs, err := c.sourceSignatures(ctx, unparsedToplevel, options,
		"Getting image list signatures",
		"Checking if image list destination supports signatures")
	if err != nil {
		return nil, err
	}

	// If the destination is a digested reference, make a note of that, determine what digest value we're
	// expecting, and check that the source manifest matches it.
	destIsDigestedReference := false
	if named := c.dest.Reference().DockerReference(); named != nil {
		if digested, ok := named.(reference.Digested); ok {
			destIsDigestedReference = true
			matches, err := manifest.MatchesDigest(manifestList, digested.Digest())
			if err != nil {
				return nil, fmt.Errorf("computing digest of source image's manifest: %w", err)
			}
			if !matches {
				return nil, errors.New("Digest of source image's manifest would not match destination reference")
			}
		}
	}

	// Determine if we're allowed to modify the manifest list.
	// If we can, set to the empty string. If we can't, set to the reason why.
	// Compare, and perhaps keep in sync with, the version in copyOneImage.
	cannotModifyManifestListReason := ""
	if len(sigs) > 0 {
		cannotModifyManifestListReason = "Would invalidate signatures"
	}
	if destIsDigestedReference {
		cannotModifyManifestListReason = "Destination specifies a digest"
	}
	if options.PreserveDigests {
		cannotModifyManifestListReason = "Instructed to preserve digests"
	}

	// Determine if we'll need to convert the manifest list to a different format.
	forceListMIMEType := options.ForceManifestMIMEType
	switch forceListMIMEType {
	case manifest.DockerV2Schema1MediaType, manifest.DockerV2Schema1SignedMediaType, manifest.DockerV2Schema2MediaType:
		forceListMIMEType = manifest.DockerV2ListMediaType
	case imgspecv1.MediaTypeImageManifest:
		forceListMIMEType = imgspecv1.MediaTypeImageIndex
	}
	selectedListType, otherManifestMIMETypeCandidates, err := c.determineListConversion(manifestType, c.dest.SupportedManifestMIMETypes(), forceListMIMEType)
	if err != nil {
		return nil, fmt.Errorf("determining manifest list type to write to destination: %w", err)
	}
	if selectedListType != originalList.MIMEType() {
		if cannotModifyManifestListReason != "" {
			return nil, fmt.Errorf("Manifest list must be converted to type %q to be written to destination, but we cannot modify it: %q", selectedListType, cannotModifyManifestListReason)
		}
	}

	// Copy each image, or just the ones we want to copy, in turn.
	instanceDigests := updatedList.Instances()
	imagesToCopy := len(instanceDigests)
	if options.ImageListSelection == CopySpecificImages {
		imagesToCopy = len(options.Instances)
	}
	c.Printf("Copying %d of %d images in list\n", imagesToCopy, len(instanceDigests))
	updates := make([]manifest.ListUpdate, len(instanceDigests))
	instancesCopied := 0
	for i, instanceDigest := range instanceDigests {
		if options.ImageListSelection == CopySpecificImages &&
			!slices.Contains(options.Instances, instanceDigest) {
			update, err := updatedList.Instance(instanceDigest)
			if err != nil {
				return nil, err
			}
			logrus.Debugf("Skipping instance %s (%d/%d)", instanceDigest, i+1, len(instanceDigests))
			// Record the digest/size/type of the manifest that we didn't copy.
			updates[i] = update
			continue
		}
		logrus.Debugf("Copying instance %s (%d/%d)", instanceDigest, i+1, len(instanceDigests))
		c.Printf("Copying image %s (%d/%d)\n", instanceDigest, instancesCopied+1, imagesToCopy)
		unparsedInstance := image.UnparsedInstance(c.rawSource, &instanceDigest)
		updatedManifest, updatedManifestType, updatedManifestDigest, err := c.copyOneImage(ctx, policyContext, options, unparsedToplevel, unparsedInstance, &instanceDigest)
		if err != nil {
			return nil, fmt.Errorf("copying image %d/%d from manifest list: %w", instancesCopied+1, imagesToCopy, err)
		}
		instancesCopied++
		// Record the result of a possible conversion here.
		update := manifest.ListUpdate{
			Digest:    updatedManifestDigest,
			Size:      int64(len(updatedManifest)),
			MediaType: updatedManifestType,
		}
		updates[i] = update
	}

	// Now reset the digest/size/types of the manifests in the list to account for any conversions that we made.
	if err = updatedList.UpdateInstances(updates); err != nil {
		return nil, fmt.Errorf("updating manifest list: %w", err)
	}

	// Iterate through supported list types, preferred format first.
	c.Printf("Writing manifest list to image destination\n")
	var errs []string
	for _, thisListType := range append([]string{selectedListType}, otherManifestMIMETypeCandidates...) {
		var attemptedList internalManifest.ListPublic = updatedList

		logrus.Debugf("Trying to use manifest list type %s…", thisListType)

		// Perform the list conversion, if we need one.
		if thisListType != updatedList.MIMEType() {
			attemptedList, err = updatedList.ConvertToMIMEType(thisListType)
			if err != nil {
				return nil, fmt.Errorf("converting manifest list to list with MIME type %q: %w", thisListType, err)
			}
		}

		// Check if the updates or a type conversion meaningfully changed the list of images
		// by serializing them both so that we can compare them.
		attemptedManifestList, err := attemptedList.Serialize()
		if err != nil {
			return nil, fmt.Errorf("encoding updated manifest list (%q: %#v): %w", updatedList.MIMEType(), updatedList.Instances(), err)
		}
		originalManifestList, err := originalList.Serialize()
		if err != nil {
			return nil, fmt.Errorf("encoding original manifest list for comparison (%q: %#v): %w", originalList.MIMEType(), originalList.Instances(), err)
		}

		// If we can't just use the original value, but we have to change it, flag an error.
		if !bytes.Equal(attemptedManifestList, originalManifestList) {
			if cannotModifyManifestListReason != "" {
				return nil, fmt.Errorf("Manifest list must be converted to type %q to be written to destination, but we cannot modify it: %q", thisListType, cannotModifyManifestListReason)
			}
			logrus.Debugf("Manifest list has been updated")
		} else {
			// We can just use the original value, so use it instead of the one we just rebuilt, so that we don't change the digest.
			attemptedManifestList = manifestList
		}

		// Save the manifest list.
		err = c.dest.PutManifest(ctx, attemptedManifestList, nil)
		if err != nil {
			logrus.Debugf("Upload of manifest list type %s failed: %v", thisListType, err)
			errs = append(errs, fmt.Sprintf("%s(%v)", thisListType, err))
			continue
		}
		errs = nil
		manifestList = attemptedManifestList
		break
	}
	if errs != nil {
		return nil, fmt.Errorf("Uploading manifest list failed, attempted the following formats: %s", strings.Join(errs, ", "))
	}

	// Sign the manifest list.
	newSigs, err := c.createSignatures(ctx, manifestList, options.SignIdentity)
	if err != nil {
		return nil, err
	}
	sigs = append(sigs, newSigs...)

	c.Printf("Storing list signatures\n")
	if err := c.dest.PutSignaturesWithFormat(ctx, sigs, nil); err != nil {
		return nil, fmt.Errorf("writing signatures: %w", err)
	}

	return manifestList, nil
}

// Printf writes a formatted string to c.reportWriter.
// Note that the method name Printf is not entirely arbitrary: (go tool vet)
// has a built-in list of functions/methods (whatever object they are for)
// which have their format strings checked; for other names we would have
// to pass a parameter to every (go tool vet) invocation.
func (c *copier) Printf(format string, a ...any) {
	fmt.Fprintf(c.reportWriter, format, a...)
}

// isTTY returns true if the io.Writer is a file and a tty.
func isTTY(w io.Writer) bool {
	if f, ok := w.(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}

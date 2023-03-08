package copy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/containers/image/v5/docker/reference"
	internalblobinfocache "github.com/containers/image/v5/internal/blobinfocache"
	"github.com/containers/image/v5/internal/image"
	"github.com/containers/image/v5/internal/imagedestination"
	"github.com/containers/image/v5/internal/imagesource"
	internalManifest "github.com/containers/image/v5/internal/manifest"
	"github.com/containers/image/v5/internal/pkg/platform"
	"github.com/containers/image/v5/internal/private"
	"github.com/containers/image/v5/internal/set"
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
	"github.com/vbauerster/mpb/v8"
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

// imageCopier tracks state specific to a single image (possibly an item of a manifest list)
type imageCopier struct {
	c                          *copier
	manifestUpdates            *types.ManifestUpdateOptions
	src                        *image.SourcedImage
	diffIDsAreNeeded           bool
	cannotModifyManifestReason string // The reason the manifest cannot be modified, or an empty string if it can
	canSubstituteBlobs         bool
	ociEncryptLayers           *[]int
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

// compareImageDestinationManifestEqual compares the `src` and `dest` image manifests (reading the manifest from the
// (possibly remote) destination). Returning true and the destination's manifest, type and digest if they compare equal.
func compareImageDestinationManifestEqual(ctx context.Context, options *Options, src *image.SourcedImage, targetInstance *digest.Digest, dest types.ImageDestination) (bool, []byte, string, digest.Digest, error) {
	srcManifestDigest, err := manifest.Digest(src.ManifestBlob)
	if err != nil {
		return false, nil, "", "", fmt.Errorf("calculating manifest digest: %w", err)
	}

	destImageSource, err := dest.Reference().NewImageSource(ctx, options.DestinationCtx)
	if err != nil {
		logrus.Debugf("Unable to create destination image %s source: %v", dest.Reference(), err)
		return false, nil, "", "", nil
	}

	destManifest, destManifestType, err := destImageSource.GetManifest(ctx, targetInstance)
	if err != nil {
		logrus.Debugf("Unable to get destination image %s/%s manifest: %v", destImageSource, targetInstance, err)
		return false, nil, "", "", nil
	}

	destManifestDigest, err := manifest.Digest(destManifest)
	if err != nil {
		return false, nil, "", "", fmt.Errorf("calculating manifest digest: %w", err)
	}

	logrus.Debugf("Comparing source and destination manifest digests: %v vs. %v", srcManifestDigest, destManifestDigest)
	if srcManifestDigest != destManifestDigest {
		return false, nil, "", "", nil
	}

	// Destination and source manifests, types and digests should all be equivalent
	return true, destManifest, destManifestType, destManifestDigest, nil
}

// Printf writes a formatted string to c.reportWriter.
// Note that the method name Printf is not entirely arbitrary: (go tool vet)
// has a built-in list of functions/methods (whatever object they are for)
// which have their format strings checked; for other names we would have
// to pass a parameter to every (go tool vet) invocation.
func (c *copier) Printf(format string, a ...any) {
	fmt.Fprintf(c.reportWriter, format, a...)
}

// checkImageDestinationForCurrentRuntime enforces dest.MustMatchRuntimeOS, if necessary.
func checkImageDestinationForCurrentRuntime(ctx context.Context, sys *types.SystemContext, src types.Image, dest types.ImageDestination) error {
	if dest.MustMatchRuntimeOS() {
		c, err := src.OCIConfig(ctx)
		if err != nil {
			return fmt.Errorf("parsing image configuration: %w", err)
		}
		wantedPlatforms, err := platform.WantedPlatforms(sys)
		if err != nil {
			return fmt.Errorf("getting current platform information %#v: %w", sys, err)
		}

		options := newOrderedSet()
		match := false
		for _, wantedPlatform := range wantedPlatforms {
			// Waiting for https://github.com/opencontainers/image-spec/pull/777 :
			// This currently can’t use image.MatchesPlatform because we don’t know what to use
			// for image.Variant.
			if wantedPlatform.OS == c.OS && wantedPlatform.Architecture == c.Architecture {
				match = true
				break
			}
			options.append(fmt.Sprintf("%s+%s", wantedPlatform.OS, wantedPlatform.Architecture))
		}
		if !match {
			logrus.Infof("Image operating system mismatch: image uses OS %q+architecture %q, expecting one of %q",
				c.OS, c.Architecture, strings.Join(options.list, ", "))
		}
	}
	return nil
}

// updateEmbeddedDockerReference handles the Docker reference embedded in Docker schema1 manifests.
func (ic *imageCopier) updateEmbeddedDockerReference() error {
	if ic.c.dest.IgnoresEmbeddedDockerReference() {
		return nil // Destination would prefer us not to update the embedded reference.
	}
	destRef := ic.c.dest.Reference().DockerReference()
	if destRef == nil {
		return nil // Destination does not care about Docker references
	}
	if !ic.src.EmbeddedDockerReferenceConflicts(destRef) {
		return nil // No reference embedded in the manifest, or it matches destRef already.
	}

	if ic.cannotModifyManifestReason != "" {
		return fmt.Errorf("Copying a schema1 image with an embedded Docker reference to %s (Docker reference %s) would change the manifest, which we cannot do: %q",
			transports.ImageName(ic.c.dest.Reference()), destRef.String(), ic.cannotModifyManifestReason)
	}
	ic.manifestUpdates.EmbeddedDockerReference = destRef
	return nil
}

func (ic *imageCopier) noPendingManifestUpdates() bool {
	return reflect.DeepEqual(*ic.manifestUpdates, types.ManifestUpdateOptions{InformationOnly: ic.manifestUpdates.InformationOnly})
}

// isTTY returns true if the io.Writer is a file and a tty.
func isTTY(w io.Writer) bool {
	if f, ok := w.(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}

// copyLayers copies layers from ic.src/ic.c.rawSource to dest, using and updating ic.manifestUpdates if necessary and ic.cannotModifyManifestReason == "".
func (ic *imageCopier) copyLayers(ctx context.Context) error {
	srcInfos := ic.src.LayerInfos()
	numLayers := len(srcInfos)
	updatedSrcInfos, err := ic.src.LayerInfosForCopy(ctx)
	if err != nil {
		return err
	}
	srcInfosUpdated := false
	// If we only need to check authorization, no updates required.
	if updatedSrcInfos != nil && !reflect.DeepEqual(srcInfos, updatedSrcInfos) {
		if ic.cannotModifyManifestReason != "" {
			return fmt.Errorf("Copying this image would require changing layer representation, which we cannot do: %q", ic.cannotModifyManifestReason)
		}
		srcInfos = updatedSrcInfos
		srcInfosUpdated = true
	}

	type copyLayerData struct {
		destInfo types.BlobInfo
		diffID   digest.Digest
		err      error
	}

	// The manifest is used to extract the information whether a given
	// layer is empty.
	man, err := manifest.FromBlob(ic.src.ManifestBlob, ic.src.ManifestMIMEType)
	if err != nil {
		return err
	}
	manifestLayerInfos := man.LayerInfos()

	// copyGroup is used to determine if all layers are copied
	copyGroup := sync.WaitGroup{}

	data := make([]copyLayerData, numLayers)
	copyLayerHelper := func(index int, srcLayer types.BlobInfo, toEncrypt bool, pool *mpb.Progress, srcRef reference.Named) {
		defer ic.c.concurrentBlobCopiesSemaphore.Release(1)
		defer copyGroup.Done()
		cld := copyLayerData{}
		if !ic.c.downloadForeignLayers && ic.c.dest.AcceptsForeignLayerURLs() && len(srcLayer.URLs) != 0 {
			// DiffIDs are, currently, needed only when converting from schema1.
			// In which case src.LayerInfos will not have URLs because schema1
			// does not support them.
			if ic.diffIDsAreNeeded {
				cld.err = errors.New("getting DiffID for foreign layers is unimplemented")
			} else {
				cld.destInfo = srcLayer
				logrus.Debugf("Skipping foreign layer %q copy to %s", cld.destInfo.Digest, ic.c.dest.Reference().Transport().Name())
			}
		} else {
			cld.destInfo, cld.diffID, cld.err = ic.copyLayer(ctx, srcLayer, toEncrypt, pool, index, srcRef, manifestLayerInfos[index].EmptyLayer)
		}
		data[index] = cld
	}

	// Decide which layers to encrypt
	layersToEncrypt := set.New[int]()
	var encryptAll bool
	if ic.ociEncryptLayers != nil {
		encryptAll = len(*ic.ociEncryptLayers) == 0
		totalLayers := len(srcInfos)
		for _, l := range *ic.ociEncryptLayers {
			// if layer is negative, it is reverse indexed.
			layersToEncrypt.Add((totalLayers + l) % totalLayers)
		}

		if encryptAll {
			for i := 0; i < len(srcInfos); i++ {
				layersToEncrypt.Add(i)
			}
		}
	}

	if err := func() error { // A scope for defer
		progressPool := ic.c.newProgressPool()
		defer progressPool.Wait()

		// Ensure we wait for all layers to be copied. progressPool.Wait() must not be called while any of the copyLayerHelpers interact with the progressPool.
		defer copyGroup.Wait()

		for i, srcLayer := range srcInfos {
			err = ic.c.concurrentBlobCopiesSemaphore.Acquire(ctx, 1)
			if err != nil {
				// This can only fail with ctx.Err(), so no need to blame acquiring the semaphore.
				return fmt.Errorf("copying layer: %w", err)
			}
			copyGroup.Add(1)
			go copyLayerHelper(i, srcLayer, layersToEncrypt.Contains(i), progressPool, ic.c.rawSource.Reference().DockerReference())
		}

		// A call to copyGroup.Wait() is done at this point by the defer above.
		return nil
	}(); err != nil {
		return err
	}

	destInfos := make([]types.BlobInfo, numLayers)
	diffIDs := make([]digest.Digest, numLayers)
	for i, cld := range data {
		if cld.err != nil {
			return cld.err
		}
		destInfos[i] = cld.destInfo
		diffIDs[i] = cld.diffID
	}

	// WARNING: If you are adding new reasons to change ic.manifestUpdates, also update the
	// OptimizeDestinationImageAlreadyExists short-circuit conditions
	ic.manifestUpdates.InformationOnly.LayerInfos = destInfos
	if ic.diffIDsAreNeeded {
		ic.manifestUpdates.InformationOnly.LayerDiffIDs = diffIDs
	}
	if srcInfosUpdated || layerDigestsDiffer(srcInfos, destInfos) {
		ic.manifestUpdates.LayerInfos = destInfos
	}
	return nil
}

// layerDigestsDiffer returns true iff the digests in a and b differ (ignoring sizes and possible other fields)
func layerDigestsDiffer(a, b []types.BlobInfo) bool {
	return !slices.EqualFunc(a, b, func(a, b types.BlobInfo) bool {
		return a.Digest == b.Digest
	})
}

// copyUpdatedConfigAndManifest updates the image per ic.manifestUpdates, if necessary,
// stores the resulting config and manifest to the destination, and returns the stored manifest
// and its digest.
func (ic *imageCopier) copyUpdatedConfigAndManifest(ctx context.Context, instanceDigest *digest.Digest) ([]byte, digest.Digest, error) {
	var pendingImage types.Image = ic.src
	if !ic.noPendingManifestUpdates() {
		if ic.cannotModifyManifestReason != "" {
			return nil, "", fmt.Errorf("Internal error: copy needs an updated manifest but that was known to be forbidden: %q", ic.cannotModifyManifestReason)
		}
		if !ic.diffIDsAreNeeded && ic.src.UpdatedImageNeedsLayerDiffIDs(*ic.manifestUpdates) {
			// We have set ic.diffIDsAreNeeded based on the preferred MIME type returned by determineManifestConversion.
			// So, this can only happen if we are trying to upload using one of the other MIME type candidates.
			// Because UpdatedImageNeedsLayerDiffIDs is true only when converting from s1 to s2, this case should only arise
			// when ic.c.dest.SupportedManifestMIMETypes() includes both s1 and s2, the upload using s1 failed, and we are now trying s2.
			// Supposedly s2-only registries do not exist or are extremely rare, so failing with this error message is good enough for now.
			// If handling such registries turns out to be necessary, we could compute ic.diffIDsAreNeeded based on the full list of manifest MIME type candidates.
			return nil, "", fmt.Errorf("Can not convert image to %s, preparing DiffIDs for this case is not supported", ic.manifestUpdates.ManifestMIMEType)
		}
		pi, err := ic.src.UpdatedImage(ctx, *ic.manifestUpdates)
		if err != nil {
			return nil, "", fmt.Errorf("creating an updated image manifest: %w", err)
		}
		pendingImage = pi
	}
	man, _, err := pendingImage.Manifest(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("reading manifest: %w", err)
	}

	if err := ic.copyConfig(ctx, pendingImage); err != nil {
		return nil, "", err
	}

	ic.c.Printf("Writing manifest to image destination\n")
	manifestDigest, err := manifest.Digest(man)
	if err != nil {
		return nil, "", err
	}
	if instanceDigest != nil {
		instanceDigest = &manifestDigest
	}
	if err := ic.c.dest.PutManifest(ctx, man, instanceDigest); err != nil {
		logrus.Debugf("Error %v while writing manifest %q", err, string(man))
		return nil, "", fmt.Errorf("writing manifest: %w", err)
	}
	return man, manifestDigest, nil
}

// copyConfig copies config.json, if any, from src to dest.
func (ic *imageCopier) copyConfig(ctx context.Context, src types.Image) error {
	srcInfo := src.ConfigInfo()
	if srcInfo.Digest != "" {
		if err := ic.c.concurrentBlobCopiesSemaphore.Acquire(ctx, 1); err != nil {
			// This can only fail with ctx.Err(), so no need to blame acquiring the semaphore.
			return fmt.Errorf("copying config: %w", err)
		}
		defer ic.c.concurrentBlobCopiesSemaphore.Release(1)

		destInfo, err := func() (types.BlobInfo, error) { // A scope for defer
			progressPool := ic.c.newProgressPool()
			defer progressPool.Wait()
			bar := ic.c.createProgressBar(progressPool, false, srcInfo, "config", "done")
			defer bar.Abort(false)
			ic.c.printCopyInfo("config", srcInfo)

			configBlob, err := src.ConfigBlob(ctx)
			if err != nil {
				return types.BlobInfo{}, fmt.Errorf("reading config blob %s: %w", srcInfo.Digest, err)
			}

			destInfo, err := ic.copyBlobFromStream(ctx, bytes.NewReader(configBlob), srcInfo, nil, true, false, bar, -1, false)
			if err != nil {
				return types.BlobInfo{}, err
			}

			bar.mark100PercentComplete()
			return destInfo, nil
		}()
		if err != nil {
			return err
		}
		if destInfo.Digest != srcInfo.Digest {
			return fmt.Errorf("Internal error: copying uncompressed config blob %s changed digest to %s", srcInfo.Digest, destInfo.Digest)
		}
	}
	return nil
}

// diffIDResult contains both a digest value and an error from diffIDComputationGoroutine.
// We could also send the error through the pipeReader, but this more cleanly separates the copying of the layer and the DiffID computation.
type diffIDResult struct {
	digest digest.Digest
	err    error
}

// copyLayer copies a layer with srcInfo (with known Digest and Annotations and possibly known Size) in src to dest, perhaps (de/re/)compressing it,
// and returns a complete blobInfo of the copied layer, and a value for LayerDiffIDs if diffIDIsNeeded
// srcRef can be used as an additional hint to the destination during checking whether a layer can be reused but srcRef can be nil.
func (ic *imageCopier) copyLayer(ctx context.Context, srcInfo types.BlobInfo, toEncrypt bool, pool *mpb.Progress, layerIndex int, srcRef reference.Named, emptyLayer bool) (types.BlobInfo, digest.Digest, error) {
	// If the srcInfo doesn't contain compression information, try to compute it from the
	// MediaType, which was either read from a manifest by way of LayerInfos() or constructed
	// by LayerInfosForCopy(), if it was supplied at all.  If we succeed in copying the blob,
	// the BlobInfo we return will be passed to UpdatedImage() and then to UpdateLayerInfos(),
	// which uses the compression information to compute the updated MediaType values.
	// (Sadly UpdatedImage() is documented to not update MediaTypes from
	//  ManifestUpdateOptions.LayerInfos[].MediaType, so we are doing it indirectly.)
	//
	// This MIME type → compression mapping belongs in manifest-specific code in our manifest
	// package (but we should preferably replace/change UpdatedImage instead of productizing
	// this workaround).
	if srcInfo.CompressionAlgorithm == nil {
		switch srcInfo.MediaType {
		case manifest.DockerV2Schema2LayerMediaType, imgspecv1.MediaTypeImageLayerGzip:
			srcInfo.CompressionAlgorithm = &compression.Gzip
		case imgspecv1.MediaTypeImageLayerZstd:
			srcInfo.CompressionAlgorithm = &compression.Zstd
		}
	}

	ic.c.printCopyInfo("blob", srcInfo)

	cachedDiffID := ic.c.blobInfoCache.UncompressedDigest(srcInfo.Digest) // May be ""
	diffIDIsNeeded := ic.diffIDsAreNeeded && cachedDiffID == ""
	// When encrypting to decrypting, only use the simple code path. We might be able to optimize more
	// (e.g. if we know the DiffID of an encrypted compressed layer, it might not be necessary to pull, decrypt and decompress again),
	// but it’s not trivially safe to do such things, so until someone takes the effort to make a comprehensive argument, let’s not.
	encryptingOrDecrypting := toEncrypt || (isOciEncrypted(srcInfo.MediaType) && ic.c.ociDecryptConfig != nil)
	canAvoidProcessingCompleteLayer := !diffIDIsNeeded && !encryptingOrDecrypting

	// Don’t read the layer from the source if we already have the blob, and optimizations are acceptable.
	if canAvoidProcessingCompleteLayer {
		canChangeLayerCompression := ic.src.CanChangeLayerCompression(srcInfo.MediaType)
		logrus.Debugf("Checking if we can reuse blob %s: general substitution = %v, compression for MIME type %q = %v",
			srcInfo.Digest, ic.canSubstituteBlobs, srcInfo.MediaType, canChangeLayerCompression)
		canSubstitute := ic.canSubstituteBlobs && ic.src.CanChangeLayerCompression(srcInfo.MediaType)
		// TODO: at this point we don't know whether or not a blob we end up reusing is compressed using an algorithm
		// that is acceptable for use on layers in the manifest that we'll be writing later, so if we end up reusing
		// a blob that's compressed with e.g. zstd, but we're only allowed to write a v2s2 manifest, this will cause
		// a failure when we eventually try to update the manifest with the digest and MIME type of the reused blob.
		// Fixing that will probably require passing more information to TryReusingBlob() than the current version of
		// the ImageDestination interface lets us pass in.
		reused, blobInfo, err := ic.c.dest.TryReusingBlobWithOptions(ctx, srcInfo, private.TryReusingBlobOptions{
			Cache:         ic.c.blobInfoCache,
			CanSubstitute: canSubstitute,
			EmptyLayer:    emptyLayer,
			LayerIndex:    &layerIndex,
			SrcRef:        srcRef,
		})
		if err != nil {
			return types.BlobInfo{}, "", fmt.Errorf("trying to reuse blob %s at destination: %w", srcInfo.Digest, err)
		}
		if reused {
			logrus.Debugf("Skipping blob %s (already present):", srcInfo.Digest)
			func() { // A scope for defer
				bar := ic.c.createProgressBar(pool, false, types.BlobInfo{Digest: blobInfo.Digest, Size: 0}, "blob", "skipped: already exists")
				defer bar.Abort(false)
				bar.mark100PercentComplete()
			}()

			// Throw an event that the layer has been skipped
			if ic.c.progress != nil && ic.c.progressInterval > 0 {
				ic.c.progress <- types.ProgressProperties{
					Event:    types.ProgressEventSkipped,
					Artifact: srcInfo,
				}
			}

			// If the reused blob has the same digest as the one we asked for, but
			// the transport didn't/couldn't supply compression info, fill it in based
			// on what we know from the srcInfos we were given.
			// If the srcInfos came from LayerInfosForCopy(), then UpdatedImage() will
			// call UpdateLayerInfos(), which uses this information to compute the
			// MediaType value for the updated layer infos, and it the transport
			// didn't pass the information along from its input to its output, then
			// it can derive the MediaType incorrectly.
			if blobInfo.Digest == srcInfo.Digest && blobInfo.CompressionAlgorithm == nil {
				blobInfo.CompressionOperation = srcInfo.CompressionOperation
				blobInfo.CompressionAlgorithm = srcInfo.CompressionAlgorithm
			}
			return blobInfo, cachedDiffID, nil
		}
	}

	// A partial pull is managed by the destination storage, that decides what portions
	// of the source file are not known yet and must be fetched.
	// Attempt a partial only when the source allows to retrieve a blob partially and
	// the destination has support for it.
	if canAvoidProcessingCompleteLayer && ic.c.rawSource.SupportsGetBlobAt() && ic.c.dest.SupportsPutBlobPartial() {
		if reused, blobInfo := func() (bool, types.BlobInfo) { // A scope for defer
			bar := ic.c.createProgressBar(pool, true, srcInfo, "blob", "done")
			hideProgressBar := true
			defer func() { // Note that this is not the same as defer bar.Abort(hideProgressBar); we need hideProgressBar to be evaluated lazily.
				bar.Abort(hideProgressBar)
			}()

			proxy := blobChunkAccessorProxy{
				wrapped: ic.c.rawSource,
				bar:     bar,
			}
			info, err := ic.c.dest.PutBlobPartial(ctx, &proxy, srcInfo, ic.c.blobInfoCache)
			if err == nil {
				if srcInfo.Size != -1 {
					bar.SetRefill(srcInfo.Size - bar.Current())
				}
				bar.mark100PercentComplete()
				hideProgressBar = false
				logrus.Debugf("Retrieved partial blob %v", srcInfo.Digest)
				return true, info
			}
			logrus.Debugf("Failed to retrieve partial blob: %v", err)
			return false, types.BlobInfo{}
		}(); reused {
			return blobInfo, cachedDiffID, nil
		}
	}

	// Fallback: copy the layer, computing the diffID if we need to do so
	return func() (types.BlobInfo, digest.Digest, error) { // A scope for defer
		bar := ic.c.createProgressBar(pool, false, srcInfo, "blob", "done")
		defer bar.Abort(false)

		srcStream, srcBlobSize, err := ic.c.rawSource.GetBlob(ctx, srcInfo, ic.c.blobInfoCache)
		if err != nil {
			return types.BlobInfo{}, "", fmt.Errorf("reading blob %s: %w", srcInfo.Digest, err)
		}
		defer srcStream.Close()

		blobInfo, diffIDChan, err := ic.copyLayerFromStream(ctx, srcStream, types.BlobInfo{Digest: srcInfo.Digest, Size: srcBlobSize, MediaType: srcInfo.MediaType, Annotations: srcInfo.Annotations}, diffIDIsNeeded, toEncrypt, bar, layerIndex, emptyLayer)
		if err != nil {
			return types.BlobInfo{}, "", err
		}

		diffID := cachedDiffID
		if diffIDIsNeeded {
			select {
			case <-ctx.Done():
				return types.BlobInfo{}, "", ctx.Err()
			case diffIDResult := <-diffIDChan:
				if diffIDResult.err != nil {
					return types.BlobInfo{}, "", fmt.Errorf("computing layer DiffID: %w", diffIDResult.err)
				}
				logrus.Debugf("Computed DiffID %s for layer %s", diffIDResult.digest, srcInfo.Digest)
				// Don’t record any associations that involve encrypted data. This is a bit crude,
				// some blob substitutions (replacing pulls of encrypted data with local reuse of known decryption outcomes)
				// might be safe, but it’s not trivially obvious, so let’s be conservative for now.
				// This crude approach also means we don’t need to record whether a blob is encrypted
				// in the blob info cache (which would probably be necessary for any more complex logic),
				// and the simplicity is attractive.
				if !encryptingOrDecrypting {
					// This is safe because we have just computed diffIDResult.Digest ourselves, and in the process
					// we have read all of the input blob, so srcInfo.Digest must have been validated by digestingReader.
					ic.c.blobInfoCache.RecordDigestUncompressedPair(srcInfo.Digest, diffIDResult.digest)
				}
				diffID = diffIDResult.digest
			}
		}

		bar.mark100PercentComplete()
		return blobInfo, diffID, nil
	}()
}

// copyLayerFromStream is an implementation detail of copyLayer; mostly providing a separate “defer” scope.
// it copies a blob with srcInfo (with known Digest and Annotations and possibly known Size) from srcStream to dest,
// perhaps (de/re/)compressing the stream,
// and returns a complete blobInfo of the copied blob and perhaps a <-chan diffIDResult if diffIDIsNeeded, to be read by the caller.
func (ic *imageCopier) copyLayerFromStream(ctx context.Context, srcStream io.Reader, srcInfo types.BlobInfo,
	diffIDIsNeeded bool, toEncrypt bool, bar *progressBar, layerIndex int, emptyLayer bool) (types.BlobInfo, <-chan diffIDResult, error) {
	var getDiffIDRecorder func(compressiontypes.DecompressorFunc) io.Writer // = nil
	var diffIDChan chan diffIDResult

	err := errors.New("Internal error: unexpected panic in copyLayer") // For pipeWriter.CloseWithbelow
	if diffIDIsNeeded {
		diffIDChan = make(chan diffIDResult, 1) // Buffered, so that sending a value after this or our caller has failed and exited does not block.
		pipeReader, pipeWriter := io.Pipe()
		defer func() { // Note that this is not the same as {defer pipeWriter.CloseWithError(err)}; we need err to be evaluated lazily.
			_ = pipeWriter.CloseWithError(err) // CloseWithError(nil) is equivalent to Close(), always returns nil
		}()

		getDiffIDRecorder = func(decompressor compressiontypes.DecompressorFunc) io.Writer {
			// If this fails, e.g. because we have exited and due to pipeWriter.CloseWithError() above further
			// reading from the pipe has failed, we don’t really care.
			// We only read from diffIDChan if the rest of the flow has succeeded, and when we do read from it,
			// the return value includes an error indication, which we do check.
			//
			// If this gets never called, pipeReader will not be used anywhere, but pipeWriter will only be
			// closed above, so we are happy enough with both pipeReader and pipeWriter to just get collected by GC.
			go diffIDComputationGoroutine(diffIDChan, pipeReader, decompressor) // Closes pipeReader
			return pipeWriter
		}
	}

	blobInfo, err := ic.copyBlobFromStream(ctx, srcStream, srcInfo, getDiffIDRecorder, false, toEncrypt, bar, layerIndex, emptyLayer) // Sets err to nil on success
	return blobInfo, diffIDChan, err
	// We need the defer … pipeWriter.CloseWithError() to happen HERE so that the caller can block on reading from diffIDChan
}

// diffIDComputationGoroutine reads all input from layerStream, uncompresses using decompressor if necessary, and sends its digest, and status, if any, to dest.
func diffIDComputationGoroutine(dest chan<- diffIDResult, layerStream io.ReadCloser, decompressor compressiontypes.DecompressorFunc) {
	result := diffIDResult{
		digest: "",
		err:    errors.New("Internal error: unexpected panic in diffIDComputationGoroutine"),
	}
	defer func() { dest <- result }()
	defer layerStream.Close() // We do not care to bother the other end of the pipe with other failures; we send them to dest instead.

	result.digest, result.err = computeDiffID(layerStream, decompressor)
}

// computeDiffID reads all input from layerStream, uncompresses it using decompressor if necessary, and returns its digest.
func computeDiffID(stream io.Reader, decompressor compressiontypes.DecompressorFunc) (digest.Digest, error) {
	if decompressor != nil {
		s, err := decompressor(stream)
		if err != nil {
			return "", err
		}
		defer s.Close()
		stream = s
	}

	return digest.Canonical.FromReader(stream)
}

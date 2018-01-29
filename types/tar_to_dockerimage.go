package types

import (
	"context"

	"github.com/containers/image/docker/reference"
	"github.com/opencontainers/image-spec/specs-go/v1"
)

type FromFsImage struct {
	unparsedImage UnparsedImage
	blobs         []BlobInfo
	metadata      map[string]string
	//u_blobs []UncompressedBlobInfo?
}

func (i FromFsImage) ConfigInfo() {

}

func (i FromFsImage) ConfigBlob([]byte) {

}

func (i FromFsImage) OCIConfig() {

}

func (i FromFsImage) LayerInfos() []BlobInfo {

}

func (i FromFSImage) EmbeddedDockerReferenceConflicts(ref reference.Named) bool {

}

// Image is the primary API for inspecting properties of images.
// An Image is based on a pair of (ImageSource, instance digest); it can represent either a manifest list or a single image instance.
//
// The Image must not be used after the underlying ImageSource is Close()d.
type Image interface {
	// Note that Reference may return nil in the return value of UpdatedImage!
	UnparsedImage
	// ConfigInfo returns a complete BlobInfo for the separate config object, or a BlobInfo{Digest:""} if there isn't a separate object.
	// Note that the config object may not exist in the underlying storage in the return value of UpdatedImage! Use ConfigBlob() below.
	ConfigInfo() BlobInfo
	// ConfigBlob returns the blob described by ConfigInfo, if ConfigInfo().Digest != ""; nil otherwise.
	// The result is cached; it is OK to call this however often you need.
	ConfigBlob() ([]byte, error)
	// OCIConfig returns the image configuration as per OCI v1 image-spec. Information about
	// layers in the resulting configuration isn't guaranteed to be returned to due how
	// old image manifests work (docker v2s1 especially).
	OCIConfig() (*v1.Image, error)
	// LayerInfos returns a list of BlobInfos of layers referenced by this image, in order (the root layer first, and then successive layered layers).
	// The Digest field is guaranteed to be provided, Size may be -1 and MediaType may be optionally provided.
	// WARNING: The list may contain duplicates, and they are semantically relevant.
	LayerInfos() []BlobInfo
	// EmbeddedDockerReferenceConflicts whether a Docker reference embedded in the manifest, if any, conflicts with destination ref.
	// It returns false if the manifest does not embed a Docker reference.
	// (This embedding unfortunately happens for Docker schema1, please do not add support for this in any new formats.)
	EmbeddedDockerReferenceConflicts(ref reference.Named) bool
	// Inspect returns various information for (skopeo inspect) parsed from the manifest and configuration.
	Inspect() (*ImageInspectInfo, error)
	// UpdatedImageNeedsLayerDiffIDs returns true iff UpdatedImage(options) needs InformationOnly.LayerDiffIDs.
	// This is a horribly specific interface, but computing InformationOnly.LayerDiffIDs can be very expensive to compute
	// (most importantly it forces us to download the full layers even if they are already present at the destination).
	UpdatedImageNeedsLayerDiffIDs(options ManifestUpdateOptions) bool
	// UpdatedImage returns a types.Image modified according to options.
	// Everything in options.InformationOnly should be provided, other fields should be set only if a modification is desired.
	// This does not change the state of the original Image object.
	UpdatedImage(options ManifestUpdateOptions) (Image, error)
	// Size returns an approximation of the amount of disk space which is consumed by the image in its current
	// location.  If the size is not known, -1 will be returned.
	Size() (int64, error)
}

// UnparsedImage is an Image-to-be; until it is verified and accepted, it only caries its identity and caches manifest and signature blobs.
// Thus, an UnparsedImage can be created from an ImageSource simply by fetching blobs without interpreting them,
// allowing cryptographic signature verification to happen first, before even fetching the manifest, or parsing anything else.
// This also makes the UnparsedImage→Image conversion an explicitly visible step.
//
// An UnparsedImage is a pair of (ImageSource, instance digest); it can represent either a manifest list or a single image instance.
//
// The UnparsedImage must not be used after the underlying ImageSource is Close()d.
type UnparsedImage interface {
	// Reference returns the reference used to set up this source, _as specified by the user_
	// (not as the image itself, or its underlying storage, claims).  This can be used e.g. to determine which public keys are trusted for this image.
	Reference() ImageReference
	// Manifest is like ImageSource.GetManifest, but the result is cached; it is OK to call this however often you need.
	Manifest() ([]byte, string, error)
	// Signatures is like ImageSource.GetSignatures, but the result is cached; it is OK to call this however often you need.
	Signatures(ctx context.Context) ([][]byte, error)
	// LayerInfosForCopy returns either nil (meaning the values in the manifest are fine), or updated values for the layer blobsums that are listed in the image's manifest.
	// The Digest field is guaranteed to be provided, Size may be -1 and MediaType may be optionally provided.
	// WARNING: The list may contain duplicates, and they are semantically relevant.
	LayerInfosForCopy() []BlobInfo
}

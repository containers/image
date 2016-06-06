package types

import (
	"io"
	"time"
)

// Registry is a service providing repositories.
type Registry interface {
	Repositories() []Repository
	Repository(ref string) Repository
	Lookup(term string) []Image // docker registry v1 only AFAICT, v2 can be built hacking with Images()
}

// Repository is a set of images.
type Repository interface {
	Images() []Image
	Image(ref string) Image // ref == image name w/o registry part
}

// ImageSource is a service, possibly remote (= slow), to download components of a single image.
// This is primarily useful for copying images around; for examining their properties, Image (below)
// is usually more useful.
type ImageSource interface {
	// IntendedDockerReference returns the full, unambiguous, Docker reference for this image, _as specified by the user_
	// (not as the image itself, or its underlying storage, claims).  This can be used e.g. to determine which public keys are trusted for this image.
	// May be "" if unknown.
	IntendedDockerReference() string
	// GetManifest returns the image's manifest along with its MIME type. The empty string is returned if the MIME type is unknown. The slice parameter indicates the supported mime types the manifest should be when getting it.
	// It may use a remote (= slow) service.
	GetManifest([]string) ([]byte, string, error)
	// Note: Calling GetLayer() may have ordering dependencies WRT other methods of this type. FIXME: How does this work with (docker save) on stdin?
	GetLayer(digest string) (io.ReadCloser, error)
	// GetSignatures returns the image's signatures.  It may use a remote (= slow) service.
	GetSignatures() ([][]byte, error)
	// Delete image from registry, if operation is supported
	Delete() error
}

// ImageDestination is a service, possibly remote (= slow), to store components of a single image.
type ImageDestination interface {
	// CanonicalDockerReference returns the full, unambiguous, Docker reference for this image (even if the user referred to the image using some shorthand notation).
	CanonicalDockerReference() (string, error)
	// FIXME? This should also receive a MIME type if known, to differentiate between schema versions.
	PutManifest([]byte) error
	// Note: Calling PutLayer() and other methods may have ordering dependencies WRT other methods of this type. FIXME: Figure out and document.
	PutLayer(digest string, stream io.Reader) error
	PutSignatures(signatures [][]byte) error
}

// Image is the primary API for inspecting properties of images.
type Image interface {
	// ref to repository?
	// IntendedDockerReference returns the full, unambiguous, Docker reference for this image, _as specified by the user_
	// (not as the image itself, or its underlying storage, claims).  This can be used e.g. to determine which public keys are trusted for this image.
	// May be "" if unknown.
	IntendedDockerReference() string
	// Manifest is like ImageSource.GetManifest, but the result is cached; it is OK to call this however often you need.
	// FIXME? This should also return a MIME type if known, to differentiate between schema versions.
	Manifest() ([]byte, error)
	// Signatures is like ImageSource.GetSignatures, but the result is cached; it is OK to call this however often you need.
	Signatures() ([][]byte, error)
	Layers(layers ...string) error // configure download directory? Call it DownloadLayers?
	// Inspect returns various information for (skopeo inspect) parsed from the manifest and configuration.
	Inspect() (*ImageInspectInfo, error)
	DockerTar() ([]byte, error) // ??? also, configure output directory
}

// ImageInspectInfo is a set of metadata describing Docker images, primarily their manifest and configuration.
type ImageInspectInfo struct {
	Tag           string
	Created       time.Time
	DockerVersion string
	Labels        map[string]string
	Architecture  string
	Os            string
	Layers        []string
}

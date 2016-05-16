package types

import (
	"fmt"
	"io"
	"time"
)

const (
	// AtomicPrefix is the URL-like schema prefix used for Atomic registry image references.
	AtomicPrefix = "atomic:"
	// DockerPrefix is the URL-like schema prefix used for Docker image references.
	DockerPrefix = "docker://"
	// DirectoryPrefix is the URL-like schema prefix used for local directories (for debugging)
	DirectoryPrefix = "dir:"
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
type ImageSource interface {
	// IntendedDockerReference returns the full, unambiguous, Docker reference for this image, _as specified by the user_
	// (not as the image itself, or its underlying storage, claims).  This can be used e.g. to determine which public keys are trusted for this image.
	// May be "" if unknown.
	IntendedDockerReference() string
	// GetManifest returns the image's manifest.  It may use a remote (= slow) service.
	GetManifest() ([]byte, error)
	GetLayer(digest string) (io.ReadCloser, error)
	// GetSignatures returns the image's signatures.  It may use a remote (= slow) service.
	GetSignatures() ([][]byte, error)
}

// ImageDestination is a service, possibly remote (= slow), to store components of a single image.
type ImageDestination interface {
	// CanonicalDockerReference returns the full, unambiguous, Docker reference for this image (even if the user referred to the image using some shorthand notation).
	CanonicalDockerReference() (string, error)
	PutManifest([]byte) error
	PutLayer(digest string, stream io.Reader) error
	PutSignatures(signatures [][]byte) error
}

// Image is a Docker image in a repository.
type Image interface {
	// ref to repository?
	// IntendedDockerReference returns the full, unambiguous, Docker reference for this image, _as specified by the user_
	// (not as the image itself, or its underlying storage, claims).  This can be used e.g. to determine which public keys are trusted for this image.
	// May be "" if unknown.
	IntendedDockerReference() string
	// Manifest is like ImageSource.GetManifest, but the result is cached; it is OK to call this however often you need.
	Manifest() ([]byte, error)
	// Signatures is like ImageSource.GetSignatures, but the result is cached; it is OK to call this however often you need.
	Signatures() ([][]byte, error)
	Layers(layers ...string) error // configure download directory? Call it DownloadLayers?
	Inspect() (*DockerImageManifest, error)
	DockerTar() ([]byte, error) // ??? also, configure output directory
	// GetRepositoryTags list all tags available in the repository. Note that this has no connection with the tag(s) used for this specific image, if any.
	GetRepositoryTags() ([]string, error)
}

// DockerImageManifest is a set of metadata describing Docker images and their manifest.json files.
// Note that this is not exactly manifest.json, e.g. some fields have been added.
type DockerImageManifest struct {
	Name          string
	Tag           string
	Created       time.Time
	DockerVersion string
	Labels        map[string]string
	Architecture  string
	Os            string
	Layers        []string
}

func (m *DockerImageManifest) String() string {
	return fmt.Sprintf("%s:%s", m.Name, m.Tag)
}

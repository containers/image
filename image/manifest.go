package image

import (
	"errors"
	"fmt"
	"time"

	"github.com/containers/image/manifest"
	"github.com/containers/image/types"
)

type config struct {
	Labels map[string]string
}

type v1Image struct {
	// Config is the configuration of the container received from the client
	Config *config `json:"config,omitempty"`
	// DockerVersion specifies version on which image is built
	DockerVersion string `json:"docker_version,omitempty"`
	// Created timestamp when image was created
	Created time.Time `json:"created"`
	// Architecture is the hardware that the image is build and runs on
	Architecture string `json:"architecture,omitempty"`
	// OS is the operating system used to build and run the image
	OS string `json:"os,omitempty"`
}

// genericManifest is an interface for parsing, modifying image manifests and related data.
// Note that the public methods are intended to be a subset of types.Image
// so that embedding a genericManifest into structs works.
// will support v1 one day...
type genericManifest interface {
	config() ([]byte, error)
	// ConfigInfo returns a complete BlobInfo for the separate config object, or a BlobInfo{Digest:""} if there isn't a separate object.
	ConfigInfo() types.BlobInfo
	// LayerInfos returns a list of BlobInfos of layers referenced by this image, in order (the root layer first, and then successive layered layers).
	// The Digest field is guaranteed to be provided; Size may be -1.
	// WARNING: The list may contain duplicates, and they are semantically relevant.
	LayerInfos() []types.BlobInfo
	imageInspectInfo() (*types.ImageInspectInfo, error) // The caller will need to fill in Layers
	// UpdatedManifest returns the image's manifest modified according to options.
	// This does not change the state of the Image object.
	UpdatedManifest(types.ManifestUpdateOptions) ([]byte, error)
}

func manifestInstanceFromBlob(src types.ImageSource, manblob []byte, mt string) (genericManifest, error) {
	switch mt {
	// "application/json" is a valid v2s1 value per https://github.com/docker/distribution/blob/master/docs/spec/manifest-v2-1.md .
	// This works for now, when nothing else seems to return "application/json"; if that were not true, the mapping/detection might
	// need to happen within the ImageSource.
	case manifest.DockerV2Schema1MediaType, manifest.DockerV2Schema1SignedMediaType, "application/json":
		return manifestSchema1FromManifest(manblob)
	case manifest.DockerV2Schema2MediaType:
		return manifestSchema2FromManifest(src, manblob)
	case manifest.DockerV2ListMediaType:
		return manifestSchema2FromManifestList(src, manblob)
	case "":
		return nil, errors.New("could not guess manifest media type")
	default:
		return nil, fmt.Errorf("unsupported manifest media type %s", mt)
	}
}

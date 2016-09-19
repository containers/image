// Package image consolidates knowledge about various container image formats
// (as opposed to image storage mechanisms, which are handled by types.ImageSource)
// and exposes all of them using an unified interface.
package image

import (
	"errors"
	"fmt"
	"time"

	"github.com/containers/image/manifest"
	"github.com/containers/image/types"
)

// genericImage is a general set of utilities for working with container images,
// whatever is their underlying location (i.e. dockerImageSource-independent).
// Note the existence of skopeo/docker.Image: some instances of a `types.Image`
// may not be a `genericImage` directly. However, most users of `types.Image`
// do not care, and those who care about `skopeo/docker.Image` know they do.
type genericImage struct {
	src types.ImageSource
	// private cache for Manifest(); nil if not yet known.
	cachedManifest []byte
	// private cache for the manifest media type w/o having to guess it
	// this may be the empty string in case the MIME Type wasn't guessed correctly
	// this field is valid only if cachedManifest is not nil
	cachedManifestMIMEType string
	// private cache for Signatures(); nil if not yet known.
	cachedSignatures [][]byte
}

// FromSource returns a types.Image implementation for source.
// The caller must call .Close() on the returned Image.
//
// FromSource “takes ownership” of the input ImageSource and will call src.Close()
// when the image is closed.  (This does not prevent callers from using both the
// Image and ImageSource objects simultaneously, but it means that they only need to
// the Image.)
func FromSource(src types.ImageSource) types.Image {
	return &genericImage{src: src}
}

// Reference returns the reference used to set up this source, _as specified by the user_
// (not as the image itself, or its underlying storage, claims).  This can be used e.g. to determine which public keys are trusted for this image.
func (i *genericImage) Reference() types.ImageReference {
	return i.src.Reference()
}

// Close removes resources associated with an initialized Image, if any.
func (i *genericImage) Close() {
	i.src.Close()
}

// Manifest is like ImageSource.GetManifest, but the result is cached; it is OK to call this however often you need.
// NOTE: It is essential for signature verification that Manifest returns the manifest from which ConfigInfo and LayerInfos is computed.
func (i *genericImage) Manifest() ([]byte, string, error) {
	if i.cachedManifest == nil {
		m, mt, err := i.src.GetManifest()
		if err != nil {
			return nil, "", err
		}
		i.cachedManifest = m
		if mt == "" || mt == "text/plain" {
			// Crane registries can return "text/plain".
			// This makes no real sense, but it happens
			// because requests for manifests are
			// redirected to a content distribution
			// network which is configured that way.
			mt = manifest.GuessMIMEType(i.cachedManifest)
		}
		i.cachedManifestMIMEType = mt
	}
	return i.cachedManifest, i.cachedManifestMIMEType, nil
}

// Signatures is like ImageSource.GetSignatures, but the result is cached; it is OK to call this however often you need.
func (i *genericImage) Signatures() ([][]byte, error) {
	if i.cachedSignatures == nil {
		sigs, err := i.src.GetSignatures()
		if err != nil {
			return nil, err
		}
		i.cachedSignatures = sigs
	}
	return i.cachedSignatures, nil
}

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

// will support v1 one day...
type genericManifest interface {
	Config() ([]byte, error)
	ConfigInfo() types.BlobInfo
	LayerInfos() []types.BlobInfo
	ImageInspectInfo() (*types.ImageInspectInfo, error) // The caller will need to fill in Layers
}

// getParsedManifest parses the manifest into a data structure, cleans it up, and returns it.
// NOTE: The manifest may have been modified in the process; DO NOT reserialize and store the return value
// if you want to preserve the original manifest; use the blob returned by Manifest() directly.
// NOTE: It is essential for signature verification that the object is computed from the same manifest which is returned by Manifest().
func (i *genericImage) getParsedManifest() (genericManifest, error) {
	manblob, mt, err := i.Manifest()
	if err != nil {
		return nil, err
	}
	switch mt {
	// "application/json" is a valid v2s1 value per https://github.com/docker/distribution/blob/master/docs/spec/manifest-v2-1.md .
	// This works for now, when nothing else seems to return "application/json"; if that were not true, the mapping/detection might
	// need to happen within the ImageSource.
	case manifest.DockerV2Schema1MediaType, manifest.DockerV2Schema1SignedMediaType, "application/json":
		return manifestSchema1FromManifest(manblob)
	case manifest.DockerV2Schema2MediaType:
		return manifestSchema2FromManifest(i.src, manblob)
	case "":
		return nil, errors.New("could not guess manifest media type")
	default:
		return nil, fmt.Errorf("unsupported manifest media type %s", mt)
	}
}

func (i *genericImage) Inspect() (*types.ImageInspectInfo, error) {
	// TODO(runcom): unused version param for now, default to docker v2-1
	m, err := i.getParsedManifest()
	if err != nil {
		return nil, err
	}
	info, err := m.ImageInspectInfo()
	if err != nil {
		return nil, err
	}
	layers := m.LayerInfos()
	info.Layers = make([]string, len(layers))
	for i, layer := range layers {
		info.Layers[i] = layer.Digest
	}
	return info, nil
}

// ConfigInfo returns a complete BlobInfo for the separate config object, or a BlobInfo{Digest:""} if there isn't a separate object.
// NOTE: It is essential for signature verification that ConfigInfo is computed from the same manifest which is returned by Manifest().
func (i *genericImage) ConfigInfo() (types.BlobInfo, error) {
	m, err := i.getParsedManifest()
	if err != nil {
		return types.BlobInfo{}, err
	}
	return m.ConfigInfo(), nil
}

// LayerInfos returns a list of BlobInfos of layers referenced by this image, in order (the root layer first, and then successive layered layers).
// The Digest field is guaranteed to be provided; Size may be -1.
// NOTE: It is essential for signature verification that LayerInfos is computed from the same manifest which is returned by Manifest().
// WARNING: The list may contain duplicates, and they are semantically relevant.
func (i *genericImage) LayerInfos() ([]types.BlobInfo, error) {
	m, err := i.getParsedManifest()
	if err != nil {
		return nil, err
	}
	return m.LayerInfos(), nil
}

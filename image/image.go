// Package image consolidates knowledge about various container image formats
// (as opposed to image storage mechanisms, which are handled by types.ImageSource)
// and exposes all of them using an unified interface.
package image

import (
	"github.com/containers/image/manifest"
	"github.com/containers/image/types"
)

// FromSource returns a types.Image implementation for source.
// The caller must call .Close() on the returned Image.
//
// FromSource “takes ownership” of the input ImageSource and will call src.Close()
// when the image is closed.  (This does not prevent callers from using both the
// Image and ImageSource objects simultaneously, but it means that they only need to
// the Image.)
//
// NOTE: If any kind of signature verification should happen, build an UnparsedImage from the value returned by NewImageSource,
// verify that UnparsedImage, and convert it into a real Image via image.FromUnparsedImage instead of calling this function.
func FromSource(src types.ImageSource) types.Image {
	return FromUnparsedImage(UnparsedFromSource(src))
}

// genericImage is a general set of utilities for working with container images,
// whatever is their underlying location (i.e. dockerImageSource-independent).
// Note the existence of skopeo/docker.Image: some instances of a `types.Image`
// may not be a `genericImage` directly. However, most users of `types.Image`
// do not care, and those who care about `skopeo/docker.Image` know they do.
type genericImage struct {
	*UnparsedImage
	trueManifestMIMETypeSet bool   // A private cache for Manifest
	trueManifestMIMEType    string // A private cache for Manifest, valid only if trueManifestMIMETypeSet
}

// FromUnparsedImage returns a types.Image implementation for unparsed.
// The caller must call .Close() on the returned Image.
//
// FromSource “takes ownership” of the input UnparsedImage and will call uparsed.Close()
// when the image is closed.  (This does not prevent callers from using both the
// UnparsedImage and ImageSource objects simultaneously, but it means that they only need to
// keep a reference to the Image.)
func FromUnparsedImage(unparsed *UnparsedImage) types.Image {
	// Note that the input parameter above is specifically *image.UnparsedImage, not types.UnparsedImage:
	// we want to be able to use unparsed.src.  We could make that an explicit interface, but, well,
	// this is the only UnparsedImage implementation around, anyway.

	// Also, we do not explicitly implement types.Image.Close; we let the implementation fall through to
	// unparsed.Close.

	// NOTE: It is essential for signature verification that all parsing done in this object happens on the same manifest which is returned by unparsed.Manifest().
	return &genericImage{
		UnparsedImage:           unparsed,
		trueManifestMIMETypeSet: false,
		trueManifestMIMEType:    "",
	}
}

// Manifest overrides the UnparsedImage.Manifest to add guessing and overrides, which we don't want to do before signature verification.
func (i *genericImage) Manifest() ([]byte, string, error) {
	m, mt, err := i.UnparsedImage.Manifest()
	if err != nil {
		return nil, "", err
	}
	if !i.trueManifestMIMETypeSet {
		if mt == "" || mt == "text/plain" {
			// Crane registries can return "text/plain".
			// This makes no real sense, but it happens
			// because requests for manifests are
			// redirected to a content distribution
			// network which is configured that way.
			mt = manifest.GuessMIMEType(i.cachedManifest)
		}
		i.trueManifestMIMEType = mt
		i.trueManifestMIMETypeSet = true
	}
	return m, mt, nil
}

// getParsedManifest parses the manifest into a data structure, cleans it up, and returns it.
// NOTE: The manifest may have been modified in the process; DO NOT reserialize and store the return value
// if you want to preserve the original manifest; use the blob returned by Manifest() directly.
func (i *genericImage) getParsedManifest() (genericManifest, error) {
	manblob, mt, err := i.Manifest()
	if err != nil {
		return nil, err
	}
	return manifestInstanceFromBlob(i.src, manblob, mt)
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
func (i *genericImage) ConfigInfo() (types.BlobInfo, error) {
	m, err := i.getParsedManifest()
	if err != nil {
		return types.BlobInfo{}, err
	}
	return m.ConfigInfo(), nil
}

// LayerInfos returns a list of BlobInfos of layers referenced by this image, in order (the root layer first, and then successive layered layers).
// The Digest field is guaranteed to be provided; Size may be -1.
// WARNING: The list may contain duplicates, and they are semantically relevant.
func (i *genericImage) LayerInfos() ([]types.BlobInfo, error) {
	m, err := i.getParsedManifest()
	if err != nil {
		return nil, err
	}
	return m.LayerInfos(), nil
}

// UpdatedManifest returns the image's manifest modified according to updateOptions.
// This does not change the state of the Image object.
func (i *genericImage) UpdatedManifest(options types.ManifestUpdateOptions) ([]byte, error) {
	m, err := i.getParsedManifest()
	if err != nil {
		return nil, err
	}
	return m.UpdatedManifest(options)
}

func (i *genericImage) IsMultiImage() (bool, error) {
	_, mt, err := i.Manifest()
	if err != nil {
		return false, err
	}
	return mt == manifest.DockerV2ListMediaType, nil
}

package archive

import (
	"context"

	"github.com/containers/image/v5/docker/tarfile"
	ctrImage "github.com/containers/image/v5/image"
	"github.com/containers/image/v5/types"
	"github.com/sirupsen/logrus"
)

type archiveImageSource struct {
	*tarfile.Source // Implements most of types.ImageSource
	ref             archiveReference
}

// newImageSource returns a types.ImageSource for the specified image reference.
// The caller must call .Close() on the returned ImageSource.
func newImageSource(ctx context.Context, sys *types.SystemContext, ref archiveReference) (types.ImageSource, error) {
	if ref.destinationRef != nil {
		logrus.Warnf("docker-archive: references are not supported for sources (ignoring)")
	}
	src, err := tarfile.NewSourceFromFileWithContext(sys, ref.path)
	if err != nil {
		return nil, err
	}
	return &archiveImageSource{
		Source: src,
		ref:    ref,
	}, nil
}

// Reference returns the reference used to set up this source, _as specified by the user_
// (not as the image itself, or its underlying storage, claims).  This can be used e.g. to determine which public keys are trusted for this image.
func (s *archiveImageSource) Reference() types.ImageReference {
	return s.ref
}

// MultiImageSourceItem is a reference to _one_ image in a multi-image archive.
// Note that MultiImageSourceItem implements types.ImageReference.  It's a
// long-lived object that can only be closed via it's parent MultiImageSource.
type MultiImageSourceItem struct {
	*archiveReference
	tarSource *tarfile.Source
}

// Manifest returns the tarfile.ManifestItem.
func (m *MultiImageSourceItem) Manifest() (*tarfile.ManifestItem, error) {
	items, err := m.tarSource.LoadTarManifest()
	if err != nil {
		return nil, err
	}
	return &items[0], nil
}

// NewImage returns a types.ImageCloser for this reference, possibly
// specialized for this ImageTransport.
func (m MultiImageSourceItem) NewImage(ctx context.Context, sys *types.SystemContext) (types.ImageCloser, error) {
	src, err := m.NewImageSource(ctx, sys)
	if err != nil {
		return nil, err
	}
	return ctrImage.FromSource(ctx, sys, src)
}

// NewImageSource returns a types.ImageSource for this reference.
func (m MultiImageSourceItem) NewImageSource(ctx context.Context, sys *types.SystemContext) (types.ImageSource, error) {
	return &archiveImageSource{
		Source: m.tarSource,
		ref:    *m.archiveReference,
	}, nil
}

// MultiImageSource allows for reading docker archives that includes more
// than one image.  Use Items() to extract
type MultiImageSource struct {
	path      string
	tarSource *tarfile.Source
}

// NewMultiImageSource creates a MultiImageSource for the
// specified path pointing to a docker-archive.
func NewMultiImageSource(ctx context.Context, sys *types.SystemContext, path string) (*MultiImageSource, error) {
	src, err := tarfile.NewSourceFromFileWithContext(sys, path)
	if err != nil {
		return nil, err
	}
	return &MultiImageSource{path: path, tarSource: src}, nil
}

// Close closes the underlying tarfile.
func (m *MultiImageSource) Close() error {
	return m.tarSource.Close()
}

// Items returns a MultiImageSourceItem for all manifests/images in the archive.
// Each references embeds an ImageSource pointing to the corresponding image in
// the archive.
func (m *MultiImageSource) Items() ([]MultiImageSourceItem, error) {
	items, err := m.tarSource.LoadTarManifest()
	if err != nil {
		return nil, err
	}
	references := []MultiImageSourceItem{}
	for index := range items {
		src, err := m.tarSource.FromManifest(index)
		if err != nil {
			return nil, err
		}
		newRef := MultiImageSourceItem{
			&archiveReference{path: m.path},
			src,
		}
		references = append(references, newRef)
	}
	return references, nil
}

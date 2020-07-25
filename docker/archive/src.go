package archive

import (
	"context"

	"github.com/containers/image/v5/docker/internal/tarfile"
	"github.com/containers/image/v5/types"
)

type archiveImageSource struct {
	*tarfile.Source // Implements most of types.ImageSource
	ref             archiveReference
}

// newImageSource returns a types.ImageSource for the specified image reference.
// The caller must call .Close() on the returned ImageSource.
func newImageSource(ctx context.Context, sys *types.SystemContext, ref archiveReference) (types.ImageSource, error) {
	archive, err := tarfile.NewReaderFromFile(sys, ref.path)
	if err != nil {
		return nil, err
	}
	src := tarfile.NewSource(archive, true, ref.ref, ref.sourceIndex)
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

package archive

import (
	"github.com/containers/image/v5/docker/internal/tarfile"
	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/types"
	"github.com/pkg/errors"
)

// Reader manages a single Docker archive, allows listing its contents and accessing
// individual images with less overhead than creating image references individually
// (because the archive is, if necessary, copied or decompressed only once).
type Reader struct {
	path    string // The original, user-specified path; not the maintained temporary file, if any
	archive *tarfile.Reader
}

// NewReader returns a Reader for path.
// The caller should call .Close() on the returned object.
func NewReader(sys *types.SystemContext, path string) (*Reader, error) {
	archive, err := tarfile.NewReaderFromFile(sys, path)
	if err != nil {
		return nil, err
	}
	return &Reader{
		path:    path,
		archive: archive,
	}, nil
}

// Close deletes temporary files associated with the Reader, if any.
func (r *Reader) Close() error {
	return r.archive.Close()
}

// List returns the a set of references for images in the Reader,
// grouped by the image the references point to.
// The references are valid only until the Reader is closed.
func (r *Reader) List() ([][]types.ImageReference, error) {
	res := [][]types.ImageReference{}
	for imageIndex, image := range r.archive.Manifest {
		refs := []types.ImageReference{}
		for _, tag := range image.RepoTags {
			parsedTag, err := reference.ParseNormalizedNamed(tag)
			if err != nil {
				return nil, errors.Wrapf(err, "Invalid tag %#v in manifest item @%d", tag, imageIndex)
			}
			nt, ok := parsedTag.(reference.NamedTagged)
			if !ok {
				return nil, errors.Errorf("Invalid tag %s (%s): does not contain a tag", tag, parsedTag.String())
			}
			ref, err := NewReference(r.path, nt)
			if err != nil {
				return nil, errors.Wrapf(err, "Error creating a reference for tag %#v in manifest item @%d", tag, imageIndex)
			}
			refs = append(refs, ref)
		}
		if len(refs) == 0 {
			ref, err := NewIndexReference(r.path, imageIndex)
			if err != nil {
				return nil, errors.Wrapf(err, "Error creating a reference for manifest item @%d", imageIndex)
			}
			refs = append(refs, ref)
		}
		res = append(res, refs)
	}
	return res, nil
}

package archive

import (
	"context"
	"io"
	"os"

	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/docker/tarfile"
	"github.com/containers/image/v5/types"
	"github.com/pkg/errors"
)

type archiveImageDestination struct {
	*tarfile.Destination // Implements most of types.ImageDestination
	ref                  archiveReference
	writer               io.Closer
}

func newImageDestination(sys *types.SystemContext, ref archiveReference) (types.ImageDestination, error) {
	return newArchiveImageDestination(sys, ref.path, ref.destinationRef)
}

func newArchiveImageDestination(sys *types.SystemContext, path string, ref reference.NamedTagged) (*archiveImageDestination, error) {
	// ref.path can be either a pipe or a regular file
	// in the case of a pipe, we require that we can open it for write
	// in the case of a regular file, we don't want to overwrite any pre-existing file
	// so we check for Size() == 0 below (This is racy, but using O_EXCL would also be racy,
	// only in a different way. Either way, it’s up to the user to not have two writers to the same path.)
	fh, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return nil, errors.Wrapf(err, "error opening file %q", path)
	}

	fhStat, err := fh.Stat()
	if err != nil {
		return nil, errors.Wrapf(err, "error statting file %q", path)
	}

	if fhStat.Mode().IsRegular() && fhStat.Size() != 0 {
		return nil, errors.New("docker-archive doesn't support modifying existing images")
	}

	tarDest := tarfile.NewDestinationWithContext(sys, fh, ref)
	if sys != nil && sys.DockerArchiveAdditionalTags != nil {
		tarDest.AddRepoTags(sys.DockerArchiveAdditionalTags)
	}
	return &archiveImageDestination{
		Destination: tarDest,
		ref:         archiveReference{path, ref},
		writer:      fh,
	}, nil
}

// DesiredLayerCompression indicates if layers must be compressed, decompressed or preserved
func (d *archiveImageDestination) DesiredLayerCompression() types.LayerCompression {
	return types.Decompress
}

// Reference returns the reference used to set up this destination.  Note that this should directly correspond to user's intent,
// e.g. it should use the public hostname instead of the result of resolving CNAMEs or following redirects.
func (d *archiveImageDestination) Reference() types.ImageReference {
	return d.ref
}

// Close removes resources associated with an initialized ImageDestination, if any.
func (d *archiveImageDestination) Close() error {
	return d.writer.Close()
}

// Commit marks the process of storing the image as successful and asks for the image to be persisted.
// WARNING: This does not have any transactional semantics:
// - Uploaded data MAY be visible to others before Commit() is called
// - Uploaded data MAY be removed or MAY remain around if Close() is called without Commit() (i.e. rollback is allowed but not guaranteed)
func (d *archiveImageDestination) Commit(ctx context.Context, unparsedToplevel types.UnparsedImage) error {
	return d.Destination.Commit(ctx)
}

func (m multiImageDestinationReference) NewImageDestination(_ context.Context, _ *types.SystemContext) (types.ImageDestination, error) {
	return m.dest, nil
}

// MultiImageDestinations allows for creating and writing to docker archives
// that include more than one image.
type MultiImageDestination struct {
	*archiveImageDestination
	path string
}

// multiImageDestinationReference is a types.ImageReference embedding a MultiImageDestination.
type multiImageDestinationReference struct {
	*archiveReference
	dest *MultiImageDestination
}

// NewMultiImageDestination returns a MultiImageDestination for the specified path.
func NewMultiImageDestination(sys *types.SystemContext, path string) (*MultiImageDestination, error) {
	dest, err := newArchiveImageDestination(sys, path, nil)
	if err != nil {
		return nil, err
	}
	return &MultiImageDestination{dest, path}, nil
}

// Reference returns an ImageReference embedding the MultiImageDestination.
func (m *MultiImageDestination) Reference() types.ImageReference {
	ref := &archiveReference{path: m.path}
	return &multiImageDestinationReference{ref, m}
}

// Close is a NOP.  Please use Finalize() for committing the archive and
// closing the underlying resources.
func (m *MultiImageDestination) Close() error {
	return nil
}

// Commit is a NOP.  Please use Finalize() for committing the archive and
// closing the underlying resources.
func (m *MultiImageDestination) Commit(_ context.Context, _ types.UnparsedImage) error {
	return nil
}

// Finalize commits pending data and closes the underlying tarfile.
func (m *MultiImageDestination) Finalize(ctx context.Context) (finalErr error) {
	defer func() {
		if err := m.writer.Close(); err != nil {
			if finalErr == nil {
				finalErr = err
			} else {
				finalErr = errors.Wrap(finalErr, err.Error())
			}
		}
	}()
	return m.Destination.Commit(ctx)
}

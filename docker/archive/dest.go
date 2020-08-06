package archive

import (
	"context"
	"io"

	"github.com/containers/image/v5/docker/internal/tarfile"
	"github.com/containers/image/v5/types"
)

type archiveImageDestination struct {
	*tarfile.Destination // Implements most of types.ImageDestination
	ref                  archiveReference
	archive              *tarfile.Writer
	writer               io.Closer
}

func newImageDestination(sys *types.SystemContext, ref archiveReference) (types.ImageDestination, error) {
	fh, err := openArchiveForWriting(ref.path)
	if err != nil {
		return nil, err
	}

	archive := tarfile.NewWriter(fh)
	tarDest := tarfile.NewDestination(sys, archive, ref.destinationRef)
	if sys != nil && sys.DockerArchiveAdditionalTags != nil {
		tarDest.AddRepoTags(sys.DockerArchiveAdditionalTags)
	}
	return &archiveImageDestination{
		Destination: tarDest,
		ref:         ref,
		archive:     archive,
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
	return d.archive.Close()
}

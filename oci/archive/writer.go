package archive

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/containers/image/v5/internal/tmpdir"
	"github.com/containers/image/v5/types"
	"github.com/containers/storage/pkg/archive"
)

// Writer manages an in-progress OCI archive and allows adding images to it
type Writer struct {
	// tempDir will be tarred to oci archive
	tempDir string
	// user-specified path
	path string
}

// NewWriter creates a temp directory will be tarred to oci-archive.
// The caller should call .Close() on the returned object.
func NewWriter(ctx context.Context, sys *types.SystemContext, file string) (*Writer, error) {
	dir, err := ioutil.TempDir(tmpdir.TemporaryDirectoryForBigFiles(sys), "oci")
	if err != nil {
		return nil, fmt.Errorf("error creating temp directory: %w", err)
	}
	ociWriter := &Writer{
		tempDir: dir,
		path:    file,
	}
	return ociWriter, nil
}

// NewReference returns an ImageReference that allows adding an image to Writer,
// with an optional image name
func (w *Writer) NewReference(name string) (types.ImageReference, error) {
	ref, err := newReference(w.path, name, -1, nil, w)
	if err != nil {
		return nil, fmt.Errorf("error creating image reference: %w", err)
	}
	return ref, nil
}

// Close writes all outstanding data about images to the archive, and
// releases state associated with the Writer, if any.
// It deletes any temporary files associated with the Writer.
// No more images can be added after this is called.
func (w *Writer) Close() error {
	err := tarDirectory(w.tempDir, w.path)
	if err2 := os.RemoveAll(w.tempDir); err2 != nil && err == nil {
		err = err2
	}
	return err
}

// tar converts the directory at src and saves it to dst
func tarDirectory(src, dst string) error {
	// input is a stream of bytes from the archive of the directory at path
	input, err := archive.Tar(src, archive.Uncompressed)
	if err != nil {
		return fmt.Errorf("retrieving stream of bytes from %q: %w", src, err)
	}

	// creates the tar file
	outFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("creating tar file %q: %w", dst, err)
	}
	defer outFile.Close()

	// copies the contents of the directory to the tar file
	// TODO: This can take quite some time, and should ideally be cancellable using a context.Context.
	_, err = io.Copy(outFile, input)

	return err
}

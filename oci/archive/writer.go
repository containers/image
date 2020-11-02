package archive

import (
	"io/ioutil"
	"os"

	"github.com/containers/image/v5/directory/explicitfilepath"
	"github.com/containers/image/v5/internal/tmpdir"
	"github.com/containers/image/v5/oci/layout"
	"github.com/containers/image/v5/types"
	"github.com/pkg/errors"
)

// Writer keeps the tempDir for creating oci archive and archive destination
type Writer struct {
	// tempDir will be tarred to oci archive
	tempDir string
	// user-specified path
	path string
}

// NewWriter creates a temp directory will be tarred to oci-archive.
// The caller should call .Close() on the returned object.
func NewWriter(sys *types.SystemContext, file string) (*Writer, error) {
	dir, err := ioutil.TempDir(tmpdir.TemporaryDirectoryForBigFiles(sys), "oci")
	if err != nil {
		return nil, errors.Wrapf(err, "error creating temp directory")
	}
	dst, err := explicitfilepath.ResolvePathToFullyExplicit(file)
	if err != nil {
		return nil, err
	}
	ociWriter := &Writer{
		tempDir: dir,
		path:    dst,
	}
	return ociWriter, nil
}

// NewReference returns an ImageReference that allows adding an image to Writer,
// with an optional image name
func (w *Writer) NewReference(name string) (types.ImageReference, error) {
	return layout.NewReference(w.tempDir, name)
}

// Close converts the data about images in the temp directory to the archive and
// deletes temporary files associated with the Writer
func (w *Writer) Close() error {
	err := tarDirectory(w.tempDir, w.path)
	if err2 := os.RemoveAll(w.tempDir); err2 != nil && err == nil {
		err = err2
	}
	return err
}

package tarfile

import (
	"archive/tar"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"path"

	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/internal/iolimits"
	"github.com/containers/image/v5/internal/tmpdir"
	"github.com/containers/image/v5/pkg/compression"
	"github.com/containers/image/v5/types"
	"github.com/pkg/errors"
)

// Reader is a ((docker save)-formatted) tar archive that allows random access to any component.
type Reader struct {
	path          string
	removeOnClose bool           // Remove file on close if true
	Manifest      []ManifestItem // Guaranteed to exist after the archive is created.
}

// NewReaderFromFile returns a Reader for the specified path.
// The caller should call .Close() on the returned archive when done.
func NewReaderFromFile(sys *types.SystemContext, path string) (*Reader, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrapf(err, "error opening file %q", path)
	}
	defer file.Close()

	// If the file is already not compressed we can just return the file itself
	// as a source. Otherwise we pass the stream to NewReaderFromStream.
	stream, isCompressed, err := compression.AutoDecompress(file)
	if err != nil {
		return nil, errors.Wrapf(err, "Error detecting compression for file %q", path)
	}
	defer stream.Close()
	if !isCompressed {
		return newReader(path, false)
	}
	return NewReaderFromStream(sys, stream)
}

// NewReaderFromStream returns a Reader for the specified inputStream,
// which can be either compressed or uncompressed. The caller can close the
// inputStream immediately after NewReaderFromFile returns.
// The caller should call .Close() on the returned archive when done.
func NewReaderFromStream(sys *types.SystemContext, inputStream io.Reader) (*Reader, error) {
	// Save inputStream to a temporary file
	tarCopyFile, err := ioutil.TempFile(tmpdir.TemporaryDirectoryForBigFiles(sys), "docker-tar")
	if err != nil {
		return nil, errors.Wrap(err, "error creating temporary file")
	}
	defer tarCopyFile.Close()

	succeeded := false
	defer func() {
		if !succeeded {
			os.Remove(tarCopyFile.Name())
		}
	}()

	// In order to be compatible with docker-load, we need to support
	// auto-decompression (it's also a nice quality-of-life thing to avoid
	// giving users really confusing "invalid tar header" errors).
	uncompressedStream, _, err := compression.AutoDecompress(inputStream)
	if err != nil {
		return nil, errors.Wrap(err, "Error auto-decompressing input")
	}
	defer uncompressedStream.Close()

	// Copy the plain archive to the temporary file.
	//
	// TODO: This can take quite some time, and should ideally be cancellable
	//       using a context.Context.
	if _, err := io.Copy(tarCopyFile, uncompressedStream); err != nil {
		return nil, errors.Wrapf(err, "error copying contents to temporary file %q", tarCopyFile.Name())
	}
	succeeded = true

	return newReader(tarCopyFile.Name(), true)
}

// newReader creates a Reader for the specified path and removeOnClose flag.
// The caller should call .Close() on the returned archive when done.
func newReader(path string, removeOnClose bool) (*Reader, error) {
	// This is a valid enough archive, except Manifest is not yet filled.
	r := Reader{
		path:          path,
		removeOnClose: removeOnClose,
	}
	succeeded := false
	defer func() {
		if !succeeded {
			r.Close()
		}
	}()

	// We initialize Manifest immediately when constructing the Reader instead
	// of later on-demand because every caller will need the data, and because doing it now
	// removes the need to synchronize the access/creation of the data if the archive is later
	// used from multiple goroutines to access different images.

	// FIXME? Do we need to deal with the legacy format?
	bytes, err := r.readTarComponent(manifestFileName, iolimits.MaxTarFileManifestSize)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(bytes, &r.Manifest); err != nil {
		return nil, errors.Wrap(err, "Error decoding tar manifest.json")
	}

	succeeded = true
	return &r, nil
}

// Close removes resources associated with an initialized Reader, if any.
func (r *Reader) Close() error {
	if r.removeOnClose {
		return os.Remove(r.path)
	}
	return nil
}

// chooseManifestItem selects a manifest item from r.Manifest matching (ref, sourceIndex), one or
// both of which should be (nil, -1).
func (r *Reader) chooseManifestItem(ref reference.NamedTagged, sourceIndex int) (*ManifestItem, error) {
	switch {
	case ref != nil && sourceIndex != -1:
		return nil, errors.Errorf("Internal error: Cannot have both ref %s and source index @%d",
			ref.String(), sourceIndex)

	case ref != nil:
		refString := ref.String()
		for i := range r.Manifest {
			for _, tag := range r.Manifest[i].RepoTags {
				parsedTag, err := reference.ParseNormalizedNamed(tag)
				if err != nil {
					return nil, errors.Wrapf(err, "Invalid tag %#v in manifest.json item @%d", tag, i)
				}
				if parsedTag.String() == refString {
					return &r.Manifest[i], nil
				}
			}
		}
		return nil, errors.Errorf("Tag %#v not found", refString)

	case sourceIndex != -1:
		if sourceIndex >= len(r.Manifest) {
			return nil, errors.Errorf("Invalid source index @%d, only %d manifest items available",
				sourceIndex, len(r.Manifest))
		}
		return &r.Manifest[sourceIndex], nil

	default:
		if len(r.Manifest) != 1 {
			return nil, errors.Errorf("Unexpected tar manifest.json: expected 1 item, got %d", len(r.Manifest))
		}
		return &r.Manifest[0], nil
	}
}

// tarReadCloser is a way to close the backing file of a tar.Reader when the user no longer needs the tar component.
type tarReadCloser struct {
	*tar.Reader
	backingFile *os.File
}

func (t *tarReadCloser) Close() error {
	return t.backingFile.Close()
}

// openTarComponent returns a ReadCloser for the specific file within the archive.
// This is linear scan; we assume that the tar file will have a fairly small amount of files (~layers),
// and that filesystem caching will make the repeated seeking over the (uncompressed) tarPath cheap enough.
// The caller should call .Close() on the returned stream.
func (r *Reader) openTarComponent(componentPath string) (io.ReadCloser, error) {
	f, err := os.Open(r.path)
	if err != nil {
		return nil, err
	}
	succeeded := false
	defer func() {
		if !succeeded {
			f.Close()
		}
	}()

	tarReader, header, err := findTarComponent(f, componentPath)
	if err != nil {
		return nil, err
	}
	if header == nil {
		return nil, os.ErrNotExist
	}
	if header.FileInfo().Mode()&os.ModeType == os.ModeSymlink { // FIXME: untested
		// We follow only one symlink; so no loops are possible.
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
		// The new path could easily point "outside" the archive, but we only compare it to existing tar headers without extracting the archive,
		// so we don't care.
		tarReader, header, err = findTarComponent(f, path.Join(path.Dir(componentPath), header.Linkname))
		if err != nil {
			return nil, err
		}
		if header == nil {
			return nil, os.ErrNotExist
		}
	}

	if !header.FileInfo().Mode().IsRegular() {
		return nil, errors.Errorf("Error reading tar archive component %s: not a regular file", header.Name)
	}
	succeeded = true
	return &tarReadCloser{Reader: tarReader, backingFile: f}, nil
}

// findTarComponent returns a header and a reader matching componentPath within inputFile,
// or (nil, nil, nil) if not found.
func findTarComponent(inputFile io.Reader, componentPath string) (*tar.Reader, *tar.Header, error) {
	t := tar.NewReader(inputFile)
	componentPath = path.Clean(componentPath)
	for {
		h, err := t.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		if path.Clean(h.Name) == componentPath {
			return t, h, nil
		}
	}
	return nil, nil, nil
}

// readTarComponent returns full contents of componentPath.
func (r *Reader) readTarComponent(path string, limit int) ([]byte, error) {
	file, err := r.openTarComponent(path)
	if err != nil {
		return nil, errors.Wrapf(err, "Error loading tar component %s", path)
	}
	defer file.Close()
	bytes, err := iolimits.ReadAtMost(file, limit)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

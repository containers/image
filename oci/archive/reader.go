package archive

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/containers/image/v5/internal/tmpdir"
	"github.com/containers/image/v5/oci/internal"
	"github.com/containers/image/v5/transports"
	"github.com/containers/image/v5/types"
	"github.com/containers/storage/pkg/archive"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// Reader manages the temp directory that the oci archive is untarred to and the
// manifest of the images. It allows listing its contents and accessing
// individual images with less overhead than creating image references individually
// (because the archive is, if necessary, copied or decompressed only once)
type Reader struct {
	manifest      *imgspecv1.Index
	tempDirectory string
	path          string // The original, user-specified path
}

// NewReader returns a Reader for src. The caller should call Close() on the returned object
func NewReader(ctx context.Context, sys *types.SystemContext, src string) (*Reader, error) {
	arch, err := os.Open(src)
	if err != nil {
		return nil, err
	}
	defer arch.Close()

	dst, err := os.MkdirTemp(tmpdir.TemporaryDirectoryForBigFiles(sys), "oci")
	if err != nil {
		return nil, fmt.Errorf("creating temp directory: %w", err)
	}
	succeeded := false
	defer func() {
		if !succeeded {
			os.RemoveAll(dst)
		}
	}()

	reader := Reader{
		tempDirectory: dst,
		path:          src,
	}

	if err := archive.NewDefaultArchiver().Untar(arch, dst, &archive.TarOptions{NoLchown: true}); err != nil {
		return nil, fmt.Errorf("untarring file %q: %w", dst, err)
	}

	indexJSON, err := os.Open(filepath.Join(dst, "index.json"))
	if err != nil {
		return nil, err
	}
	defer indexJSON.Close()
	reader.manifest = &imgspecv1.Index{}
	if err := json.NewDecoder(indexJSON).Decode(reader.manifest); err != nil {
		return nil, err
	}

	succeeded = true
	return &reader, nil
}

// NewReaderForReference creates a Reader from a Reader-independent imageReference, which must be from oci/archive.Transport,
// and a variant of imageReference that points at the same image within the reader.
// The caller should call .Close() on the returned Reader.
func NewReaderForReference(ctx context.Context, sys *types.SystemContext, ref types.ImageReference) (*Reader, types.ImageReference, error) {
	standalone, ok := ref.(ociArchiveReference)
	if !ok {
		return nil, nil, fmt.Errorf("Internal error: NewReader called for a non-oci/archive ImageReference %s", transports.ImageName(ref))
	}
	if standalone.archiveReader != nil {
		return nil, nil, fmt.Errorf("Internal error: NewReader called for a reader-bound reference %s", standalone.StringWithinTransport())
	}

	reader, err := NewReader(ctx, sys, standalone.resolvedFile)
	if err != nil {
		return nil, nil, err
	}

	succeeded := false
	defer func() {
		if !succeeded {
			reader.Close()
		}
	}()

	readerRef, err := newReference(standalone.resolvedFile, standalone.image, -1, reader, nil)
	if err != nil {
		return nil, nil, err
	}
	succeeded = true
	return reader, readerRef, nil
}

// ListResult wraps the image reference and the manifest for loading
type ListResult struct {
	ImageRef           types.ImageReference
	ManifestDescriptor imgspecv1.Descriptor
}

// List returns a slice of manifests included in the archive
func (r *Reader) List() ([]ListResult, error) {
	var res []ListResult

	for manifestIndex, md := range r.manifest.Manifests {
		refName := internal.NameFromAnnotations(md.Annotations)
		index := -1
		if refName == "" {
			index = manifestIndex
		}
		ref, err := newReference(r.path, refName, index, r, nil)
		if err != nil {
			return nil, fmt.Errorf("error creating image reference: %w", err)
		}
		reference := ListResult{
			ImageRef:           ref,
			ManifestDescriptor: md,
		}
		res = append(res, reference)
	}
	return res, nil
}

// Close deletes temporary files associated with the Reader, if any.
func (r *Reader) Close() error {
	return os.RemoveAll(r.tempDirectory)
}

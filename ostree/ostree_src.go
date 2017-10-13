// +build !containers_image_ostree_stub

package ostree

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"unsafe"

	"github.com/containers/storage/pkg/ioutils"
	"github.com/ostreedev/ostree-go/pkg/otbuiltin"
	"github.com/pkg/errors"
	"github.com/vbatts/tar-split/tar/asm"
	"github.com/vbatts/tar-split/tar/storage"

	"github.com/containers/image/manifest"
	"github.com/containers/image/types"
	"github.com/opencontainers/go-digest"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// #cgo pkg-config: glib-2.0 gobject-2.0
// #include <glib.h>
// #include <glib-object.h>
// #include <gio/gio.h>
// #include <stdlib.h>
import "C"

func parseGVariantString(in string) string {
	cstring := C.CString(in)
	defer C.free(unsafe.Pointer(cstring))

	ptr := (*C.char)(C.g_variant_get_string(C.g_variant_parse(nil, (*C.gchar)(cstring), nil, nil, nil), nil))
	return C.GoString(ptr)
}

type ostreeImageSource struct {
	ref    ostreeReference
	tmpDir string
}

// newImageSource returns an ImageSource for reading from an existing directory.
func newImageSource(ctx *types.SystemContext, tmpDir string, ref ostreeReference) (types.ImageSource, error) {
	return &ostreeImageSource{ref: ref, tmpDir: tmpDir}, nil
}

// Reference returns the reference used to set up this source.
func (s *ostreeImageSource) Reference() types.ImageReference {
	return s.ref
}

// Close removes resources associated with an initialized ImageSource, if any.
func (s *ostreeImageSource) Close() error {
	if s.repo != nil {
		C.g_object_unref(C.gpointer(s.repo))
	}
	return nil
}

func (s *ostreeImageSource) getLayerSize(blob string) (int64, error) {
	b := fmt.Sprintf("ociimage/%s", blob)
	// Use golang bindings once they support to read metadata
	out, err := exec.Command("ostree", "show", "--repo", s.ref.repo, "--print-metadata-key=docker.size", b).CombinedOutput()
	if err != nil {
		return 0, err
	}

	data := parseGVariantString(string(out))
	size, err := strconv.ParseInt(data, 10, 64)
	if err != nil {
		return 0, err
	}
	return size, nil
}

func (s *ostreeImageSource) getTarSplitData(blob string) ([]byte, error) {
	b := fmt.Sprintf("ociimage/%s", blob)
	// Use golang bindings once they support to read metadata
	out, err := exec.Command("ostree", "show", "--repo", s.ref.repo, "--print-metadata-key=tarsplit.output", b).CombinedOutput()
	if err != nil {
		if strings.Index(string(out), "No such metadata key") >= 0 {
			return nil, nil
		}
		return nil, err
	}
	data := parseGVariantString(string(out))
	return base64.StdEncoding.DecodeString(data)
}

// GetManifest returns the image's manifest along with its MIME type (which may be empty when it can't be determined but the manifest is available).
// It may use a remote (= slow) service.
func (s *ostreeImageSource) GetManifest(instanceDigest *digest.Digest) ([]byte, string, error) {
	if instanceDigest != nil {
		return nil, "", errors.Errorf(`Manifest lists are not supported by "ostree:"`)
	}
	b := fmt.Sprintf("ociimage/%s", s.ref.branchName)
	// Use golang bindings once they support to read metadata
	out, err := exec.Command("ostree", "show", "--repo", s.ref.repo, "--print-metadata-key=docker.manifest", b).Output()
	if err != nil {
		return nil, "", err
	}
	m := []byte(parseGVariantString(string(out)))
	return m, manifest.GuessMIMEType(m), nil
}

func (s *ostreeImageSource) GetTargetManifest(digest digest.Digest) ([]byte, string, error) {
	return nil, imgspecv1.MediaTypeImageManifest, nil
}

func (s *ostreeImageSource) checkout(dest string, checksum string) error {
	opts := otbuiltin.NewCheckoutOptions()
	opts.UserMode = true
	commit := fmt.Sprintf("ociimage/%s", checksum)
	return otbuiltin.Checkout(s.ref.repo, dest, commit, opts)
}

// GetBlob returns a stream for the specified blob, and the blob's size.
func (s *ostreeImageSource) GetBlob(info types.BlobInfo) (io.ReadCloser, int64, error) {
	blob := info.Digest.Hex()

	dir, err := ioutil.TempDir(s.tmpDir, string(info.Digest))
	if err != nil {
		return nil, 0, err
	}

	layer := fmt.Sprintf("%s/layer", dir)

	err = s.checkout(layer, blob)
	if err != nil {
		os.RemoveAll(dir)
		return nil, 0, err
	}

	layerSize, err := s.getLayerSize(blob)
	if err != nil {
		os.RemoveAll(dir)
		return nil, 0, err
	}

	tarsplit, err := s.getTarSplitData(blob)
	if err != nil {
		os.RemoveAll(dir)
		return nil, 0, err
	}

	// if tarsplit is nil we are looking at the manifest.  Return directly the file in layer/content
	if tarsplit == nil {
		stream, err := os.OpenFile(fmt.Sprintf("%s/content", layer), os.O_RDONLY, 0)
		if err != nil {
			os.RemoveAll(dir)
			return nil, 0, err
		}
		return ioutils.NewReadCloserWrapper(stream, func() error {
			return os.RemoveAll(dir)
		}), layerSize, nil
	}

	mf := bytes.NewReader(tarsplit)
	mfz, err := gzip.NewReader(mf)
	if err != nil {
		os.RemoveAll(dir)
		return nil, 0, err
	}
	defer mfz.Close()
	metaUnpacker := storage.NewJSONUnpacker(mfz)

	ots := asm.NewOutputTarStream(storage.NewPathFileGetter(layer), metaUnpacker)

	pipeReader, pipeWriter := io.Pipe()
	gzipWriter := gzip.NewWriter(pipeWriter)
	go func() {
		io.Copy(gzipWriter, ots)
		gzipWriter.Close()
		pipeWriter.Close()
	}()

	rc := ioutils.NewReadCloserWrapper(pipeReader, func() error {
		ots.Close()
		return os.RemoveAll(dir)
	})
	return rc, layerSize, nil
}

func (s *ostreeImageSource) GetSignatures(ctx context.Context, instanceDigest *digest.Digest) ([][]byte, error) {
	return [][]byte{}, nil
}

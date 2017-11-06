// +build !containers_image_ostree_stub

package ostree

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"unsafe"

	"github.com/containers/image/manifest"
	"github.com/containers/image/types"
	"github.com/containers/storage/pkg/ioutils"
	"github.com/opencontainers/go-digest"
	glib "github.com/ostreedev/ostree-go/pkg/glibobject"
	"github.com/pkg/errors"
	"github.com/vbatts/tar-split/tar/asm"
	"github.com/vbatts/tar-split/tar/storage"
)

// #cgo pkg-config: glib-2.0 gobject-2.0 ostree-1
// #include <glib.h>
// #include <glib-object.h>
// #include <gio/gio.h>
// #include <stdlib.h>
// #include <ostree.h>
// #include <gio/ginputstream.h>
import "C"

func parseGVariantString(in string) string {
	cstring := C.CString(in)
	defer C.free(unsafe.Pointer(cstring))

	variant := C.g_variant_parse(nil, (*C.gchar)(cstring), nil, nil, nil)
	defer C.g_variant_unref(variant)

	ptr := (*C.char)(C.g_variant_get_string(variant, nil))

	return C.GoString(ptr)
}

type ostreeImageSource struct {
	ref    ostreeReference
	tmpDir string
	repo   *C.struct_OstreeRepo
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
	return nil, "", errors.New("manifest lists are not supported by this transport")
}

func openRepo(path string) (*C.struct_OstreeRepo, error) {
	var cerr *C.GError
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	pathc := C.g_file_new_for_path(cpath)
	defer C.g_object_unref(C.gpointer(pathc))
	repo := C.ostree_repo_new(pathc)
	r := glib.GoBool(glib.GBoolean(C.ostree_repo_open(repo, nil, &cerr)))
	if !r {
		C.g_object_unref(C.gpointer(repo))
		return nil, glib.ConvertGError(glib.ToGError(unsafe.Pointer(cerr)))
	}
	return repo, nil
}

type ostreePathFileGetter struct {
	repo       *C.struct_OstreeRepo
	parentRoot *C.GFile
}

type ostreeReader struct {
	stream *C.GFileInputStream
}

func (o ostreeReader) Close() error {
	C.g_object_unref(C.gpointer(o.stream))
	return nil
}
func (o ostreeReader) Read(p []byte) (int, error) {
	var cerr *C.GError
	instanceCast := C.g_type_check_instance_cast((*C.GTypeInstance)(unsafe.Pointer(o.stream)), C.g_input_stream_get_type())
	stream := (*C.GInputStream)(unsafe.Pointer(instanceCast))

	b := C.g_input_stream_read_bytes(stream, (C.gsize)(cap(p)), nil, &cerr)
	if b == nil {
		return 0, glib.ConvertGError(glib.ToGError(unsafe.Pointer(cerr)))
	}
	defer C.g_bytes_unref(b)

	count := int(C.g_bytes_get_size(b))
	if count == 0 {
		return 0, io.EOF
	}
	data := (*[1 << 30]byte)(unsafe.Pointer(C.g_bytes_get_data(b, nil)))[:count:count]
	copy(p, data)
	return count, nil
}

func newOSTreePathFileGetter(repo *C.struct_OstreeRepo, commit string) (*ostreePathFileGetter, error) {
	var cerr *C.GError
	var parentRoot *C.GFile
	cCommit := C.CString(commit)
	defer C.free(unsafe.Pointer(cCommit))
	if !glib.GoBool(glib.GBoolean(C.ostree_repo_read_commit(repo, cCommit, &parentRoot, nil, nil, &cerr))) {
		return &ostreePathFileGetter{}, glib.ConvertGError(glib.ToGError(unsafe.Pointer(cerr)))
	}

	C.g_object_ref(C.gpointer(repo))

	return &ostreePathFileGetter{repo: repo, parentRoot: parentRoot}, nil
}

func (o ostreePathFileGetter) Get(filename string) (io.ReadCloser, error) {
	var file *C.GFile
	if strings.HasPrefix(filename, "./") {
		filename = filename[2:]
	}
	cfilename := C.CString(filename)
	defer C.free(unsafe.Pointer(cfilename))

	file = (*C.GFile)(C.g_file_resolve_relative_path(o.parentRoot, cfilename))

	var cerr *C.GError
	stream := C.g_file_read(file, nil, &cerr)
	if stream == nil {
		return nil, glib.ConvertGError(glib.ToGError(unsafe.Pointer(cerr)))
	}

	return &ostreeReader{stream: stream}, nil
}

func (o ostreePathFileGetter) Close() {
	C.g_object_ref(C.gpointer(o.repo))
	C.g_object_unref(C.gpointer(o.parentRoot))
}

func (s *ostreeImageSource) readSingleFile(commit, path string) (io.ReadCloser, error) {
	getter, err := newOSTreePathFileGetter(s.repo, commit)
	if err != nil {
		return nil, err
	}
	defer getter.Close()

	return getter.Get(path)
}

// GetBlob returns a stream for the specified blob, and the blob's size.
func (s *ostreeImageSource) GetBlob(info types.BlobInfo) (io.ReadCloser, int64, error) {
	blob := info.Digest.Hex()
	branch := fmt.Sprintf("ociimage/%s", blob)

	if s.repo == nil {
		repo, err := openRepo(s.ref.repo)
		if err != nil {
			return nil, 0, err
		}
		s.repo = repo
	}

	layerSize, err := s.getLayerSize(blob)
	if err != nil {
		return nil, 0, err
	}

	tarsplit, err := s.getTarSplitData(blob)
	if err != nil {
		return nil, 0, err
	}

	// if tarsplit is nil we are looking at the manifest.  Return directly the file in /content
	if tarsplit == nil {
		file, err := s.readSingleFile(branch, "/content")
		if err != nil {
			return nil, 0, err
		}
		return file, layerSize, nil
	}

	mf := bytes.NewReader(tarsplit)
	mfz, err := gzip.NewReader(mf)
	if err != nil {
		return nil, 0, err
	}
	defer mfz.Close()
	metaUnpacker := storage.NewJSONUnpacker(mfz)

	getter, err := newOSTreePathFileGetter(s.repo, branch)
	if err != nil {
		return nil, 0, err
	}

	ots := asm.NewOutputTarStream(getter, metaUnpacker)

	pipeReader, pipeWriter := io.Pipe()
	gzipWriter := gzip.NewWriter(pipeWriter)
	go func() {
		io.Copy(gzipWriter, ots)
		gzipWriter.Close()
		pipeWriter.Close()
	}()

	rc := ioutils.NewReadCloserWrapper(pipeReader, func() error {
		getter.Close()
		return ots.Close()
	})
	return rc, layerSize, nil
}

func (s *ostreeImageSource) GetSignatures(ctx context.Context, instanceDigest *digest.Digest) ([][]byte, error) {
	return [][]byte{}, nil
}

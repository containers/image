//go:build !containers_image_storage_stub
// +build !containers_image_storage_stub

package storage

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	imanifest "github.com/containers/image/v5/internal/manifest"
	"github.com/containers/image/v5/internal/private"
	"github.com/containers/image/v5/pkg/blobinfocache/memory"
	"github.com/containers/image/v5/types"
	"github.com/containers/storage"
	"github.com/containers/storage/pkg/archive"
	"github.com/containers/storage/pkg/idtools"
	"github.com/containers/storage/pkg/ioutils"
	"github.com/containers/storage/pkg/reexec"
	ddigest "github.com/opencontainers/go-digest"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	_ types.ImageDestination   = &storageImageDestination{}
	_ private.ImageDestination = (*storageImageDestination)(nil)
	_ types.ImageSource        = &storageImageSource{}
	_ private.ImageSource      = (*storageImageSource)(nil)
	_ types.ImageReference     = &storageReference{}
	_ types.ImageTransport     = &storageTransport{}
)

const (
	layerSize = 12345
)

func TestMain(m *testing.M) {
	if reexec.Init() {
		return
	}
	debug := false
	flag.BoolVar(&debug, "debug", false, "print debug statements")
	flag.Parse()
	if debug {
		logrus.SetLevel(logrus.DebugLevel)
	}
	os.Exit(m.Run())
}

func newStoreWithGraphDriverOptions(t *testing.T, options []string) storage.Store {
	wd := t.TempDir()
	run := filepath.Join(wd, "run")
	root := filepath.Join(wd, "root")
	Transport.SetDefaultUIDMap([]idtools.IDMap{{
		ContainerID: 0,
		HostID:      os.Getuid(),
		Size:        1,
	}})
	Transport.SetDefaultGIDMap([]idtools.IDMap{{
		ContainerID: 0,
		HostID:      os.Getgid(),
		Size:        1,
	}})
	store, err := storage.GetStore(storage.StoreOptions{
		RunRoot:            run,
		GraphRoot:          root,
		GraphDriverName:    "vfs",
		GraphDriverOptions: options,
		UIDMap:             Transport.DefaultUIDMap(),
		GIDMap:             Transport.DefaultGIDMap(),
	})
	if err != nil {
		t.Fatal(err)
	}
	Transport.SetStore(store)
	return store
}

func newStore(t *testing.T) storage.Store {
	return newStoreWithGraphDriverOptions(t, []string{})
}

func TestParse(t *testing.T) {
	store := newStore(t)

	ref, err := Transport.ParseReference("test")
	if err != nil {
		t.Fatalf("ParseReference(%q) returned error %v", "test", err)
	}
	if ref == nil {
		t.Fatalf("ParseReference returned nil reference")
	}

	ref, err = Transport.ParseStoreReference(store, "test")
	if err != nil {
		t.Fatalf("ParseStoreReference(%q) returned error %v", "test", err)
	}

	strRef := ref.StringWithinTransport()
	ref, err = Transport.ParseReference(strRef)
	if err != nil {
		t.Fatalf("ParseReference(%q) returned error: %v", strRef, err)
	}
	if ref == nil {
		t.Fatalf("ParseReference(%q) returned nil reference", strRef)
	}

	transport := storageTransport{
		store:         store,
		defaultUIDMap: Transport.(*storageTransport).defaultUIDMap,
		defaultGIDMap: Transport.(*storageTransport).defaultGIDMap,
	}
	_references := []storageReference{
		{
			named:     ref.(*storageReference).named,
			id:        "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			transport: transport,
		},
		{
			named:     ref.(*storageReference).named,
			transport: transport,
		},
		{
			id:        "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			transport: transport,
		},
		{
			named:     ref.DockerReference(),
			transport: transport,
		},
	}
	for _, reference := range _references {
		s := reference.StringWithinTransport()
		ref, err := Transport.ParseStoreReference(store, s)
		if err != nil {
			t.Fatalf("ParseReference(%q) returned error: %v", strRef, err)
		}
		if ref.id != reference.id {
			t.Fatalf("ParseReference(%q) failed to extract ID", s)
		}
		if reference.named == nil {
			if ref.named != nil {
				t.Fatalf("ParseReference(%q) set non-nil named", s)
			}
		} else {
			if ref.named.String() != reference.named.String() {
				t.Fatalf("ParseReference(%q) failed to extract reference (%q!=%q)", s, ref.named.String(), reference.named.String())
			}
		}
	}
}

func TestParseWithGraphDriverOptions(t *testing.T) {
	optionLists := [][]string{
		{},
		{"vfs.ignore_chown_errors=true"},
		{"vfs.ignore_chown_errors=false"},
	}
	for _, optionList := range optionLists {
		store := newStoreWithGraphDriverOptions(t, optionList)
		ref, err := Transport.ParseStoreReference(store, "test")
		require.NoError(t, err, optionList)
		require.NotNil(t, ref)
		spec := ref.StringWithinTransport()
		ref2, err := Transport.ParseReference(spec)
		require.NoError(t, err)
		require.NotNil(t, ref)
		sref, ok := ref2.(*storageReference)
		require.True(t, ok, "transport %s", ref2.Transport().Name())
		parsedOptions := sref.transport.store.GraphOptions()
		assert.Equal(t, optionList, parsedOptions)
	}
}

func systemContext() *types.SystemContext {
	return &types.SystemContext{}
}

// makeLayerGoroutine writes to pwriter, and on success, updates uncompressedCount
// before it terminates.
func makeLayerGoroutine(pwriter io.Writer, uncompressedCount *int64, compression archive.Compression) error {
	var uncompressed *ioutils.WriteCounter
	if compression != archive.Uncompressed {
		compressor, err := archive.CompressStream(pwriter, compression)
		if err != nil {
			return fmt.Errorf("compressing layer: %w", err)
		}
		defer compressor.Close()
		uncompressed = ioutils.NewWriteCounter(compressor)
	} else {
		uncompressed = ioutils.NewWriteCounter(pwriter)
	}
	twriter := tar.NewWriter(uncompressed)
	// 	defer twriter.Close()
	// should be called here to correctly terminate the archive.
	// We do not do that, to workaround https://github.com/containers/storage/issues/1729 :
	// tar-split runs a goroutine that consumes/forwards tar content and might access
	// concurrently-freed objects if it sees a valid EOF marker.
	// Instead, realy on raw EOF to terminate the goroutine.
	// This depends on implementation details of tar.Writer (that it does not do any
	// internal buffering).

	buf := make([]byte, layerSize)
	n, err := rand.Read(buf)
	if err != nil {
		return fmt.Errorf("reading tar data: %w", err)
	}
	if n != len(buf) {
		return fmt.Errorf("short read reading tar data: %d < %d", n, len(buf))
	}
	for i := 1024; i < 2048; i++ {
		buf[i] = 0
	}

	if err := twriter.WriteHeader(&tar.Header{
		Name:       "/random-single-file",
		Mode:       0600,
		Size:       int64(len(buf)),
		ModTime:    time.Now(),
		AccessTime: time.Now(),
		ChangeTime: time.Now(),
		Typeflag:   tar.TypeReg,
	}); err != nil {
		return fmt.Errorf("Error writing tar header: %w", err)
	}
	n, err = twriter.Write(buf)
	if err != nil {
		return fmt.Errorf("Error writing tar header: %w", err)
	}
	if n != len(buf) {
		return fmt.Errorf("Short write writing tar header: %d < %d", n, len(buf))
	}
	if err := twriter.Flush(); err != nil {
		return fmt.Errorf("Error flushing output to tar archive: %w", err)
	}
	*uncompressedCount = uncompressed.Count
	return nil
}

func makeLayer(t *testing.T, compression archive.Compression) (ddigest.Digest, int64, int64, []byte) {
	preader, pwriter := io.Pipe()
	var uncompressedCount int64
	go func() {
		err := errors.New("Internal error: unexpected panic in makeLayer")
		defer func() { // Note that this is not the same as {defer pipeWriter.CloseWithError(err)}; we need err to be evaluated lazily.
			_ = pwriter.CloseWithError(err)
		}()
		err = makeLayerGoroutine(pwriter, &uncompressedCount, compression)
	}()

	tbuffer := bytes.Buffer{}
	_, err := io.Copy(&tbuffer, preader)
	require.NoError(t, err)
	sum := ddigest.SHA256.FromBytes(tbuffer.Bytes())
	return sum, uncompressedCount, int64(tbuffer.Len()), tbuffer.Bytes()
}

func TestWriteRead(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("TestWriteRead requires root privileges")
	}

	config := `{"config":{"labels":{}},"created":"2006-01-02T15:04:05Z"}`
	sum := ddigest.SHA256.FromBytes([]byte(config))
	configInfo := types.BlobInfo{
		Digest: sum,
		Size:   int64(len(config)),
	}
	manifests := []string{
		//`{
		//    "schemaVersion": 2,
		//    "mediaType": "application/vnd.oci.image.manifest.v1+json",
		//    "config": {
		//	"mediaType": "application/vnd.oci.image.serialization.config.v1+json",
		//	"size": %cs,
		//	"digest": "%ch"
		//    },
		//    "layers": [
		//	{
		//	    "mediaType": "application/vnd.oci.image.serialization.rootfs.tar.gzip",
		//	    "digest": "%lh",
		//	    "size": %ls
		//	}
		//    ]
		//}`,
		`{
		    "schemaVersion": 1,
		    "name": "test",
		    "tag": "latest",
		    "architecture": "amd64",
		    "fsLayers": [
			{
			    "blobSum": "%lh"
			}
		    ],
		    "history": [
			{
				"v1Compatibility": "{\"id\":\"%li\",\"created\":\"2016-03-03T11:29:44.222098366Z\",\"container\":\"\",\"container_config\":{\"Hostname\":\"56f0fe1dfc95\",\"Domainname\":\"\",\"User\":\"\",\"AttachStdin\":false,\"AttachStdout\":false,\"AttachStderr\":false,\"ExposedPorts\":null,\"PublishService\":\"\",\"Tty\":false,\"OpenStdin\":false,\"StdinOnce\":false,\"Env\":null,\"Cmd\":[\"/bin/sh\"],\"Image\":\"\",\"Volumes\":null,\"VolumeDriver\":\"\",\"WorkingDir\":\"\",\"Entrypoint\":null,\"NetworkDisabled\":false,\"MacAddress\":\"\",\"OnBuild\":null,\"Labels\":{}},\"docker_version\":\"1.8.2-fc22\",\"author\":\"\\\"William Temple \\u003cwtemple at redhat dot com\\u003e\\\"\",\"config\":{\"Hostname\":\"56f0fe1dfc95\",\"Domainname\":\"\",\"User\":\"\",\"AttachStdin\":false,\"AttachStdout\":false,\"AttachStderr\":false,\"ExposedPorts\":null,\"PublishService\":\"\",\"Tty\":false,\"OpenStdin\":false,\"StdinOnce\":false,\"Env\":null,\"Cmd\":null,\"Image\":\"\",\"Volumes\":null,\"VolumeDriver\":\"\",\"WorkingDir\":\"\",\"Entrypoint\":null,\"NetworkDisabled\":false,\"MacAddress\":\"\",\"OnBuild\":null,\"Labels\":{}},\"architecture\":\"amd64\",\"os\":\"linux\",\"Size\":%ls}"
			}
		    ]
		}`,
		`{
		    "schemaVersion": 2,
		    "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
		    "config": {
			"mediaType": "application/vnd.docker.container.image.v1+json",
			"size": %cs,
			"digest": "%ch"
		    },
		    "layers": [
			{
			    "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
			    "digest": "%lh",
			    "size": %ls
			}
		    ]
		}`,
	}
	// Start signatures with 0xA0 to fool internal/signature.FromBlob into thinking it is valid GPG
	signatures := [][]byte{
		[]byte("\xA0Signature A"),
		[]byte("\xA0Signature B"),
	}
	newStore(t)
	ref, err := Transport.ParseReference("test")
	if err != nil {
		t.Fatalf("ParseReference(%q) returned error %v", "test", err)
	}
	if ref == nil {
		t.Fatalf("ParseReference returned nil reference")
	}
	cache := memory.New()

	for _, manifestFmt := range manifests {
		dest, err := ref.NewImageDestination(context.Background(), systemContext())
		if err != nil {
			t.Fatalf("NewImageDestination(%q) returned error %v", ref.StringWithinTransport(), err)
		}
		if dest == nil {
			t.Fatalf("NewImageDestination(%q) returned no destination", ref.StringWithinTransport())
		}
		if dest.Reference().StringWithinTransport() != ref.StringWithinTransport() {
			t.Fatalf("NewImageDestination(%q) changed the reference to %q", ref.StringWithinTransport(), dest.Reference().StringWithinTransport())
		}
		t.Logf("supported manifest MIME types: %v", dest.SupportedManifestMIMETypes())
		if err := dest.SupportsSignatures(context.Background()); err != nil {
			t.Fatalf("Destination image doesn't support signatures: %v", err)
		}
		t.Logf("compress layers: %v", dest.DesiredLayerCompression())
		compression := archive.Uncompressed
		if dest.DesiredLayerCompression() == types.Compress {
			compression = archive.Gzip
		}
		digest, decompressedSize, size, blob := makeLayer(t, compression)
		if _, err := dest.PutBlob(context.Background(), bytes.NewReader(blob), types.BlobInfo{
			Size:   size,
			Digest: digest,
		}, cache, false); err != nil {
			t.Fatalf("Error saving randomly-generated layer to destination: %v", err)
		}
		t.Logf("Wrote randomly-generated layer %q (%d/%d bytes) to destination", digest, size, decompressedSize)
		if _, err := dest.PutBlob(context.Background(), strings.NewReader(config), configInfo, cache, false); err != nil {
			t.Fatalf("Error saving config to destination: %v", err)
		}
		manifest := strings.ReplaceAll(manifestFmt, "%lh", digest.String())
		manifest = strings.ReplaceAll(manifest, "%ch", configInfo.Digest.String())
		manifest = strings.ReplaceAll(manifest, "%ls", fmt.Sprintf("%d", size))
		manifest = strings.ReplaceAll(manifest, "%cs", fmt.Sprintf("%d", configInfo.Size))
		li := digest.Hex()
		manifest = strings.ReplaceAll(manifest, "%li", li)
		manifest = strings.ReplaceAll(manifest, "%ci", sum.Hex())
		t.Logf("this manifest is %q", manifest)
		if err := dest.PutManifest(context.Background(), []byte(manifest), nil); err != nil {
			t.Fatalf("Error saving manifest to destination: %v", err)
		}
		if err := dest.PutSignatures(context.Background(), signatures, nil); err != nil {
			t.Fatalf("Error saving signatures to destination: %v", err)
		}
		unparsedToplevel := unparsedImage{
			imageReference: nil,
			manifestBytes:  []byte(manifest),
			manifestType:   imanifest.GuessMIMEType([]byte(manifest)),
			signatures:     signatures,
		}
		if err := dest.Commit(context.Background(), &unparsedToplevel); err != nil {
			t.Fatalf("Error committing changes to destination: %v", err)
		}
		dest.Close()

		img, err := ref.NewImage(context.Background(), systemContext())
		if err != nil {
			t.Fatalf("NewImage(%q) returned error %v", ref.StringWithinTransport(), err)
		}
		imageConfigInfo := img.ConfigInfo()
		if imageConfigInfo.Digest != "" {
			blob, err := img.ConfigBlob(context.Background())
			if err != nil {
				t.Fatalf("image %q claimed there was a config blob, but couldn't produce it: %v", ref.StringWithinTransport(), err)
			}
			sum := ddigest.SHA256.FromBytes(blob)
			if sum != configInfo.Digest {
				t.Fatalf("image config blob digest for %q doesn't match", ref.StringWithinTransport())
			}
			if int64(len(blob)) != configInfo.Size {
				t.Fatalf("image config size for %q changed from %d to %d", ref.StringWithinTransport(), configInfo.Size, len(blob))
			}
		}
		layerInfos := img.LayerInfos()
		if layerInfos == nil {
			t.Fatalf("image for %q returned empty layer list", ref.StringWithinTransport())
		}
		imageInfo, err := img.Inspect(context.Background())
		if err != nil {
			t.Fatalf("Inspect(%q) returned error %v", ref.StringWithinTransport(), err)
		}
		if imageInfo.Created.IsZero() {
			t.Fatalf("Image %q claims to have been created at time 0", ref.StringWithinTransport())
		}

		src, err := ref.NewImageSource(context.Background(), systemContext())
		if err != nil {
			t.Fatalf("NewImageSource(%q) returned error %v", ref.StringWithinTransport(), err)
		}
		if src == nil {
			t.Fatalf("NewImageSource(%q) returned no source", ref.StringWithinTransport())
		}
		// Note that we would strip a digest here, but not a tag.
		if src.Reference().StringWithinTransport() != ref.StringWithinTransport() {
			// As long as it's only the addition of an ID suffix, that's okay.
			if !strings.HasPrefix(src.Reference().StringWithinTransport(), ref.StringWithinTransport()+"@") {
				t.Fatalf("NewImageSource(%q) changed the reference to %q", ref.StringWithinTransport(), src.Reference().StringWithinTransport())
			}
		}
		_, manifestType, err := src.GetManifest(context.Background(), nil)
		if err != nil {
			t.Fatalf("GetManifest(%q) returned error %v", ref.StringWithinTransport(), err)
		}
		t.Logf("this manifest's type appears to be %q", manifestType)
		sum, err = imanifest.Digest([]byte(manifest))
		if err != nil {
			t.Fatalf("manifest.Digest() returned error %v", err)
		}
		retrieved, _, err := src.GetManifest(context.Background(), &sum)
		if err != nil {
			t.Fatalf("GetManifest(%q) with an instanceDigest is supposed to succeed", ref.StringWithinTransport())
		}
		if string(retrieved) != manifest {
			t.Fatalf("GetManifest(%q) with an instanceDigest retrieved a different manifest", ref.StringWithinTransport())
		}
		sigs, err := src.GetSignatures(context.Background(), nil)
		if err != nil {
			t.Fatalf("GetSignatures(%q) returned error %v", ref.StringWithinTransport(), err)
		}
		if len(sigs) < len(signatures) {
			t.Fatalf("Lost %d signatures", len(signatures)-len(sigs))
		}
		if len(sigs) > len(signatures) {
			t.Fatalf("Gained %d signatures", len(sigs)-len(signatures))
		}
		for i := range sigs {
			if !bytes.Equal(sigs[i], signatures[i]) {
				t.Fatalf("Signature %d was corrupted", i)
			}
		}
		sigs2, err := src.GetSignatures(context.Background(), &sum)
		if err != nil {
			t.Fatalf("GetSignatures(%q) with instance %s returned error %v", ref.StringWithinTransport(), sum.String(), err)
		}
		if !reflect.DeepEqual(sigs, sigs2) {
			t.Fatalf("GetSignatures(%q) with instance %s returned a different result", ref.StringWithinTransport(), sum.String())
		}
		for _, layerInfo := range layerInfos {
			buf := bytes.Buffer{}
			layer, size, err := src.GetBlob(context.Background(), layerInfo, cache)
			if err != nil {
				t.Fatalf("Error reading layer %q from %q", layerInfo.Digest, ref.StringWithinTransport())
			}
			t.Logf("Decompressing blob %q, blob size = %d, layerInfo.Size = %d bytes", layerInfo.Digest, size, layerInfo.Size)
			hasher := sha256.New()
			compressed := ioutils.NewWriteCounter(hasher)
			countedLayer := io.TeeReader(layer, compressed)
			decompressed, err := archive.DecompressStream(countedLayer)
			if err != nil {
				t.Fatalf("Error decompressing layer %q from %q", layerInfo.Digest, ref.StringWithinTransport())
			}
			n, err := io.Copy(&buf, decompressed)
			require.NoError(t, err)
			layer.Close()
			if layerInfo.Size >= 0 && compressed.Count != layerInfo.Size {
				t.Fatalf("Blob size is different than expected: %d != %d, read %d", compressed.Count, layerInfo.Size, n)
			}
			if size >= 0 && compressed.Count != size {
				t.Fatalf("Blob size mismatch: %d != %d, read %d", compressed.Count, size, n)
			}
			sum := hasher.Sum(nil)
			if ddigest.NewDigestFromBytes(ddigest.SHA256, sum) != layerInfo.Digest {
				t.Fatalf("Layer blob digest for %q doesn't match", ref.StringWithinTransport())
			}
		}
		src.Close()
		img.Close()
		err = ref.DeleteImage(context.Background(), systemContext())
		if err != nil {
			t.Fatalf("DeleteImage(%q) returned error %v", ref.StringWithinTransport(), err)
		}
	}
}

func TestDuplicateName(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("TestDuplicateName requires root privileges")
	}

	newStore(t)
	cache := memory.New()

	ref, err := Transport.ParseReference("test")
	if err != nil {
		t.Fatalf("ParseReference(%q) returned error %v", "test", err)
	}
	if ref == nil {
		t.Fatalf("ParseReference returned nil reference")
	}

	dest, err := ref.NewImageDestination(context.Background(), systemContext())
	if err != nil {
		t.Fatalf("NewImageDestination(%q, first pass) returned error %v", ref.StringWithinTransport(), err)
	}
	if dest == nil {
		t.Fatalf("NewImageDestination(%q, first pass) returned no destination", ref.StringWithinTransport())
	}
	digest, _, size, blob := makeLayer(t, archive.Uncompressed)
	if _, err := dest.PutBlob(context.Background(), bytes.NewReader(blob), types.BlobInfo{
		Size:   size,
		Digest: digest,
	}, cache, false); err != nil {
		t.Fatalf("Error saving randomly-generated layer to destination, first pass: %v", err)
	}
	manifest := fmt.Sprintf(`
	        {
		    "schemaVersion": 2,
		    "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
		    "layers": [
			{
			    "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
			    "digest": "%s",
			    "size": %d
			}
		    ]
		}
	`, digest, size)
	if err := dest.PutManifest(context.Background(), []byte(manifest), nil); err != nil {
		t.Fatalf("Error storing manifest to destination: %v", err)
	}
	unparsedToplevel := unparsedImage{
		imageReference: nil,
		manifestBytes:  []byte(manifest),
		manifestType:   imanifest.GuessMIMEType([]byte(manifest)),
		signatures:     nil,
	}
	if err := dest.Commit(context.Background(), &unparsedToplevel); err != nil {
		t.Fatalf("Error committing changes to destination, first pass: %v", err)
	}
	dest.Close()

	dest, err = ref.NewImageDestination(context.Background(), systemContext())
	if err != nil {
		t.Fatalf("NewImageDestination(%q, second pass) returned error %v", ref.StringWithinTransport(), err)
	}
	if dest == nil {
		t.Fatalf("NewImageDestination(%q, second pass) returned no destination", ref.StringWithinTransport())
	}
	digest, _, size, blob = makeLayer(t, archive.Gzip)
	if _, err := dest.PutBlob(context.Background(), bytes.NewReader(blob), types.BlobInfo{
		Size:   size,
		Digest: digest,
	}, cache, false); err != nil {
		t.Fatalf("Error saving randomly-generated layer to destination, second pass: %v", err)
	}
	manifest = fmt.Sprintf(`
	        {
		    "schemaVersion": 2,
		    "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
		    "layers": [
			{
			    "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
			    "digest": "%s",
			    "size": %d
			}
		    ]
		}
	`, digest, size)
	if err := dest.PutManifest(context.Background(), []byte(manifest), nil); err != nil {
		t.Fatalf("Error storing manifest to destination: %v", err)
	}
	unparsedToplevel = unparsedImage{
		imageReference: nil,
		manifestBytes:  []byte(manifest),
		manifestType:   imanifest.GuessMIMEType([]byte(manifest)),
		signatures:     nil,
	}
	if err := dest.Commit(context.Background(), &unparsedToplevel); err != nil {
		t.Fatalf("Error committing changes to destination, second pass: %v", err)
	}
	dest.Close()
}

func TestDuplicateID(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("TestDuplicateID requires root privileges")
	}

	newStore(t)
	cache := memory.New()

	ref, err := Transport.ParseReference("@aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatalf("ParseReference(%q) returned error %v", "test", err)
	}
	if ref == nil {
		t.Fatalf("ParseReference returned nil reference")
	}

	dest, err := ref.NewImageDestination(context.Background(), systemContext())
	if err != nil {
		t.Fatalf("NewImageDestination(%q, first pass) returned error %v", ref.StringWithinTransport(), err)
	}
	if dest == nil {
		t.Fatalf("NewImageDestination(%q, first pass) returned no destination", ref.StringWithinTransport())
	}
	digest, _, size, blob := makeLayer(t, archive.Gzip)
	if _, err := dest.PutBlob(context.Background(), bytes.NewReader(blob), types.BlobInfo{
		Size:   size,
		Digest: digest,
	}, cache, false); err != nil {
		t.Fatalf("Error saving randomly-generated layer to destination, first pass: %v", err)
	}
	manifest := fmt.Sprintf(`
	        {
		    "schemaVersion": 2,
		    "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
		    "layers": [
			{
			    "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
			    "digest": "%s",
			    "size": %d
			}
		    ]
		}
	`, digest, size)
	if err := dest.PutManifest(context.Background(), []byte(manifest), nil); err != nil {
		t.Fatalf("Error storing manifest to destination: %v", err)
	}
	unparsedToplevel := unparsedImage{
		imageReference: nil,
		manifestBytes:  []byte(manifest),
		manifestType:   imanifest.GuessMIMEType([]byte(manifest)),
		signatures:     nil,
	}
	if err := dest.Commit(context.Background(), &unparsedToplevel); err != nil {
		t.Fatalf("Error committing changes to destination, first pass: %v", err)
	}
	dest.Close()

	dest, err = ref.NewImageDestination(context.Background(), systemContext())
	if err != nil {
		t.Fatalf("NewImageDestination(%q, second pass) returned error %v", ref.StringWithinTransport(), err)
	}
	if dest == nil {
		t.Fatalf("NewImageDestination(%q, second pass) returned no destination", ref.StringWithinTransport())
	}
	digest, _, size, blob = makeLayer(t, archive.Gzip)
	if _, err := dest.PutBlob(context.Background(), bytes.NewReader(blob), types.BlobInfo{
		Size:   size,
		Digest: digest,
	}, cache, false); err != nil {
		t.Fatalf("Error saving randomly-generated layer to destination, second pass: %v", err)
	}
	manifest = fmt.Sprintf(`
	        {
		    "schemaVersion": 2,
		    "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
		    "layers": [
			{
			    "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
			    "digest": "%s",
			    "size": %d
			}
		    ]
		}
	`, digest, size)
	if err := dest.PutManifest(context.Background(), []byte(manifest), nil); err != nil {
		t.Fatalf("Error storing manifest to destination: %v", err)
	}
	unparsedToplevel = unparsedImage{
		imageReference: nil,
		manifestBytes:  []byte(manifest),
		manifestType:   imanifest.GuessMIMEType([]byte(manifest)),
		signatures:     nil,
	}
	if err := dest.Commit(context.Background(), &unparsedToplevel); !errors.Is(err, storage.ErrDuplicateID) {
		if err != nil {
			t.Fatalf("Wrong error committing changes to destination, second pass: %v", err)
		}
		t.Fatal("Incorrectly succeeded committing changes to destination, second pass: no error")
	}
	dest.Close()
}

func TestDuplicateNameID(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("TestDuplicateNameID requires root privileges")
	}

	newStore(t)
	cache := memory.New()

	ref, err := Transport.ParseReference("test@aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatalf("ParseReference(%q) returned error %v", "test", err)
	}
	if ref == nil {
		t.Fatalf("ParseReference returned nil reference")
	}

	dest, err := ref.NewImageDestination(context.Background(), systemContext())
	if err != nil {
		t.Fatalf("NewImageDestination(%q, first pass) returned error %v", ref.StringWithinTransport(), err)
	}
	if dest == nil {
		t.Fatalf("NewImageDestination(%q, first pass) returned no destination", ref.StringWithinTransport())
	}
	digest, _, size, blob := makeLayer(t, archive.Gzip)
	if _, err := dest.PutBlob(context.Background(), bytes.NewReader(blob), types.BlobInfo{
		Size:   size,
		Digest: digest,
	}, cache, false); err != nil {
		t.Fatalf("Error saving randomly-generated layer to destination, first pass: %v", err)
	}
	manifest := fmt.Sprintf(`
	        {
		    "schemaVersion": 2,
		    "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
		    "layers": [
			{
			    "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
			    "digest": "%s",
			    "size": %d
			}
		    ]
		}
	`, digest, size)
	if err := dest.PutManifest(context.Background(), []byte(manifest), nil); err != nil {
		t.Fatalf("Error storing manifest to destination: %v", err)
	}
	unparsedToplevel := unparsedImage{
		imageReference: nil,
		manifestBytes:  []byte(manifest),
		manifestType:   imanifest.GuessMIMEType([]byte(manifest)),
		signatures:     nil,
	}
	if err := dest.Commit(context.Background(), &unparsedToplevel); err != nil {
		t.Fatalf("Error committing changes to destination, first pass: %v", err)
	}
	dest.Close()

	dest, err = ref.NewImageDestination(context.Background(), systemContext())
	if err != nil {
		t.Fatalf("NewImageDestination(%q, second pass) returned error %v", ref.StringWithinTransport(), err)
	}
	if dest == nil {
		t.Fatalf("NewImageDestination(%q, second pass) returned no destination", ref.StringWithinTransport())
	}
	digest, _, size, blob = makeLayer(t, archive.Gzip)
	if _, err := dest.PutBlob(context.Background(), bytes.NewReader(blob), types.BlobInfo{
		Size:   size,
		Digest: digest,
	}, cache, false); err != nil {
		t.Fatalf("Error saving randomly-generated layer to destination, second pass: %v", err)
	}
	manifest = fmt.Sprintf(`
	        {
		    "schemaVersion": 2,
		    "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
		    "layers": [
			{
			    "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
			    "digest": "%s",
			    "size": %d
			}
		    ]
		}
	`, digest, size)
	if err := dest.PutManifest(context.Background(), []byte(manifest), nil); err != nil {
		t.Fatalf("Error storing manifest to destination: %v", err)
	}
	unparsedToplevel = unparsedImage{
		imageReference: nil,
		manifestBytes:  []byte(manifest),
		manifestType:   imanifest.GuessMIMEType([]byte(manifest)),
		signatures:     nil,
	}
	if err := dest.Commit(context.Background(), &unparsedToplevel); !errors.Is(err, storage.ErrDuplicateID) {
		if err != nil {
			t.Fatalf("Wrong error committing changes to destination, second pass: %v", err)
		}
		t.Fatal("Incorrectly succeeded committing changes to destination, second pass: no error")
	}
	dest.Close()
}

func TestNamespaces(t *testing.T) {
	newStore(t)

	ref, err := Transport.ParseReference("test@aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatalf("ParseReference(%q) returned error %v", "test", err)
	}
	if ref == nil {
		t.Fatalf("ParseReference returned nil reference")
	}

	namespaces := ref.PolicyConfigurationNamespaces()
	for _, namespace := range namespaces {
		t.Logf("namespace: %q", namespace)
		err = Transport.ValidatePolicyConfigurationScope(namespace)
		if ref == nil {
			t.Fatalf("ValidatePolicyConfigurationScope(%q) returned error: %v", namespace, err)
		}
	}
	namespace := ref.StringWithinTransport()
	t.Logf("ref: %q", namespace)
	err = Transport.ValidatePolicyConfigurationScope(namespace)
	if err != nil {
		t.Fatalf("ValidatePolicyConfigurationScope(%q) returned error: %v", namespace, err)
	}
	for _, namespace := range []string{
		"@beefee",
		":miracle",
		":miracle@beefee",
		"@beefee:miracle",
	} {
		t.Logf("invalid ref: %q", namespace)
		err = Transport.ValidatePolicyConfigurationScope(namespace)
		if err == nil {
			t.Fatalf("ValidatePolicyConfigurationScope(%q) should have failed", namespace)
		}
	}
}

func TestSize(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("TestSize requires root privileges")
	}

	config := `{"config":{"labels":{}},"created":"2006-01-02T15:04:05Z"}`
	sum := ddigest.SHA256.FromBytes([]byte(config))
	configInfo := types.BlobInfo{
		Digest: sum,
		Size:   int64(len(config)),
	}

	newStore(t)
	cache := memory.New()

	ref, err := Transport.ParseReference("test")
	if err != nil {
		t.Fatalf("ParseReference(%q) returned error %v", "test", err)
	}
	if ref == nil {
		t.Fatalf("ParseReference returned nil reference")
	}

	dest, err := ref.NewImageDestination(context.Background(), systemContext())
	if err != nil {
		t.Fatalf("NewImageDestination(%q) returned error %v", ref.StringWithinTransport(), err)
	}
	if dest == nil {
		t.Fatalf("NewImageDestination(%q) returned no destination", ref.StringWithinTransport())
	}
	if _, err := dest.PutBlob(context.Background(), strings.NewReader(config), configInfo, cache, false); err != nil {
		t.Fatalf("Error saving config to destination: %v", err)
	}
	digest1, usize1, size1, blob := makeLayer(t, archive.Gzip)
	if _, err := dest.PutBlob(context.Background(), bytes.NewReader(blob), types.BlobInfo{
		Size:   size1,
		Digest: digest1,
	}, cache, false); err != nil {
		t.Fatalf("Error saving randomly-generated layer 1 to destination: %v", err)
	}
	digest2, usize2, size2, blob := makeLayer(t, archive.Gzip)
	if _, err := dest.PutBlob(context.Background(), bytes.NewReader(blob), types.BlobInfo{
		Size:   size2,
		Digest: digest2,
	}, cache, false); err != nil {
		t.Fatalf("Error saving randomly-generated layer 2 to destination: %v", err)
	}
	manifest := fmt.Sprintf(`
	        {
		    "schemaVersion": 2,
		    "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
		    "config": {
			"mediaType": "application/vnd.docker.container.image.v1+json",
			"size": %d,
			"digest": "%s"
		    },
		    "layers": [
			{
			    "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
			    "digest": "%s",
			    "size": %d
			},
			{
			    "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
			    "digest": "%s",
			    "size": %d
			}
		    ]
		}
	`, configInfo.Size, configInfo.Digest, digest1, size1, digest2, size2)
	if err := dest.PutManifest(context.Background(), []byte(manifest), nil); err != nil {
		t.Fatalf("Error storing manifest to destination: %v", err)
	}
	unparsedToplevel := unparsedImage{
		imageReference: nil,
		manifestBytes:  []byte(manifest),
		manifestType:   imanifest.GuessMIMEType([]byte(manifest)),
		signatures:     nil,
	}
	if err := dest.Commit(context.Background(), &unparsedToplevel); err != nil {
		t.Fatalf("Error committing changes to destination: %v", err)
	}
	dest.Close()

	img, err := ref.NewImage(context.Background(), systemContext())
	if err != nil {
		t.Fatalf("NewImage(%q) returned error %v", ref.StringWithinTransport(), err)
	}
	usize, err := img.Size()
	if usize == -1 || err != nil {
		t.Fatalf("Error calculating image size: %v", err)
	}
	if int(usize) != len(config)+int(usize1)+int(usize2)+2*len(manifest) {
		t.Fatalf("Unexpected image size: %d != %d + %d + %d + %d (%d)", usize, len(config), usize1, usize2, len(manifest), len(config)+int(usize1)+int(usize2)+2*len(manifest))
	}
	img.Close()
}

func TestDuplicateBlob(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("TestDuplicateBlob requires root privileges")
	}

	config := `{"config":{"labels":{}},"created":"2006-01-02T15:04:05Z"}`
	sum := ddigest.SHA256.FromBytes([]byte(config))
	configInfo := types.BlobInfo{
		Digest: sum,
		Size:   int64(len(config)),
	}

	newStore(t)
	cache := memory.New()

	ref, err := Transport.ParseReference("test")
	if err != nil {
		t.Fatalf("ParseReference(%q) returned error %v", "test", err)
	}
	if ref == nil {
		t.Fatalf("ParseReference returned nil reference")
	}

	dest, err := ref.NewImageDestination(context.Background(), systemContext())
	if err != nil {
		t.Fatalf("NewImageDestination(%q) returned error %v", ref.StringWithinTransport(), err)
	}
	if dest == nil {
		t.Fatalf("NewImageDestination(%q) returned no destination", ref.StringWithinTransport())
	}
	digest1, _, size1, blob1 := makeLayer(t, archive.Gzip)
	if _, err := dest.PutBlob(context.Background(), bytes.NewReader(blob1), types.BlobInfo{
		Size:   size1,
		Digest: digest1,
	}, cache, false); err != nil {
		t.Fatalf("Error saving randomly-generated layer 1 to destination (first copy): %v", err)
	}
	digest2, _, size2, blob2 := makeLayer(t, archive.Gzip)
	if _, err := dest.PutBlob(context.Background(), bytes.NewReader(blob2), types.BlobInfo{
		Size:   size2,
		Digest: digest2,
	}, cache, false); err != nil {
		t.Fatalf("Error saving randomly-generated layer 2 to destination (first copy): %v", err)
	}
	if _, err := dest.PutBlob(context.Background(), bytes.NewReader(blob1), types.BlobInfo{
		Size:   size1,
		Digest: digest1,
	}, cache, false); err != nil {
		t.Fatalf("Error saving randomly-generated layer 1 to destination (second copy): %v", err)
	}
	if _, err := dest.PutBlob(context.Background(), bytes.NewReader(blob2), types.BlobInfo{
		Size:   size2,
		Digest: digest2,
	}, cache, false); err != nil {
		t.Fatalf("Error saving randomly-generated layer 2 to destination (second copy): %v", err)
	}
	manifest := fmt.Sprintf(`
	        {
		    "schemaVersion": 2,
		    "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
		    "config": {
			"mediaType": "application/vnd.docker.container.image.v1+json",
			"size": %d,
			"digest": "%s"
		    },
		    "layers": [
			{
			    "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
			    "digest": "%s",
			    "size": %d
			},
			{
			    "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
			    "digest": "%s",
			    "size": %d
			},
			{
			    "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
			    "digest": "%s",
			    "size": %d
			},
			{
			    "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
			    "digest": "%s",
			    "size": %d
			}
		    ]
		}
	`, configInfo.Size, configInfo.Digest, digest1, size1, digest2, size2, digest1, size1, digest2, size2)
	if err := dest.PutManifest(context.Background(), []byte(manifest), nil); err != nil {
		t.Fatalf("Error storing manifest to destination: %v", err)
	}
	unparsedToplevel := unparsedImage{
		imageReference: nil,
		manifestBytes:  []byte(manifest),
		manifestType:   imanifest.GuessMIMEType([]byte(manifest)),
		signatures:     nil,
	}
	if err := dest.Commit(context.Background(), &unparsedToplevel); err != nil {
		t.Fatalf("Error committing changes to destination: %v", err)
	}
	dest.Close()

	img, err := ref.NewImage(context.Background(), systemContext())
	if err != nil {
		t.Fatalf("NewImage(%q) returned error %v", ref.StringWithinTransport(), err)
	}
	src, err := ref.NewImageSource(context.Background(), systemContext())
	if err != nil {
		t.Fatalf("NewImageSource(%q) returned error %v", ref.StringWithinTransport(), err)
	}
	source, ok := src.(*storageImageSource)
	if !ok {
		t.Fatalf("ImageSource is not a storage image")
	}
	layers := []string{}
	layersInfo, err := img.LayerInfosForCopy(context.Background())
	if err != nil {
		t.Fatalf("LayerInfosForCopy() returned error %v", err)
	}
	for _, layerInfo := range layersInfo {
		digestLayers, _ := source.imageRef.transport.store.LayersByUncompressedDigest(layerInfo.Digest)
		rc, _, layerID, err := source.getBlobAndLayerID(layerInfo.Digest, digestLayers)
		if err != nil {
			t.Fatalf("getBlobAndLayerID(%q) returned error %v", layerInfo.Digest, err)
		}
		_, err = io.Copy(io.Discard, rc)
		require.NoError(t, err)
		rc.Close()
		layers = append(layers, layerID)
	}
	if len(layers) != 4 {
		t.Fatalf("Incorrect number of layers: %d", len(layers))
	}
	for i, layerID := range layers {
		for j, otherID := range layers {
			if i != j && layerID == otherID {
				t.Fatalf("Layer IDs are not unique: %v", layers)
			}
		}
	}
	src.Close()
	img.Close()
}

type unparsedImage struct {
	imageReference types.ImageReference
	manifestBytes  []byte
	manifestType   string
	signatures     [][]byte
}

func (u *unparsedImage) Reference() types.ImageReference {
	return u.imageReference
}
func (u *unparsedImage) Manifest(context.Context) ([]byte, string, error) {
	return u.manifestBytes, u.manifestType, nil
}
func (u *unparsedImage) Signatures(context.Context) ([][]byte, error) {
	return u.signatures, nil
}

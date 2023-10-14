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
	"runtime"
	"strings"
	"testing"
	"time"

	imanifest "github.com/containers/image/v5/internal/manifest"
	"github.com/containers/image/v5/internal/private"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/pkg/blobinfocache/memory"
	"github.com/containers/image/v5/types"
	"github.com/containers/storage"
	"github.com/containers/storage/pkg/archive"
	"github.com/containers/storage/pkg/idtools"
	"github.com/containers/storage/pkg/ioutils"
	"github.com/containers/storage/pkg/reexec"
	"github.com/opencontainers/go-digest"
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
	// Due to https://github.com/containers/storage/pull/811 , c/storage can be used on macOS unprivileged,
	// and this actually causes problems because the PR linked above does not exclude UID changes on some code paths.
	// This condition should be removed again if https://github.com/containers/storage/pull/1735 is merged.
	if runtime.GOOS != "darwin" {
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
	}
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

type testBlob struct {
	compressedDigest digest.Digest
	uncompressedSize int64
	compressedSize   int64
	data             []byte
}

func makeLayer(t *testing.T, compression archive.Compression) testBlob {
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
	return testBlob{
		compressedDigest: digest.SHA256.FromBytes(tbuffer.Bytes()),
		uncompressedSize: uncompressedCount,
		compressedSize:   int64(tbuffer.Len()),
		data:             tbuffer.Bytes(),
	}
}

func (l testBlob) storeBlob(t *testing.T, dest types.ImageDestination, cache types.BlobInfoCache, mimeType string) manifest.Schema2Descriptor {
	_, err := dest.PutBlob(context.Background(), bytes.NewReader(l.data), types.BlobInfo{
		Size:   l.compressedSize,
		Digest: l.compressedDigest,
	}, cache, false)
	require.NoError(t, err)
	return manifest.Schema2Descriptor{
		MediaType: mimeType,
		Size:      l.compressedSize,
		Digest:    l.compressedDigest,
	}
}

// ensureTestCanCreateImages skips the current test if it is not possible to create layers and images in a private store.
func ensureTestCanCreateImages(t *testing.T) {
	t.Helper()
	switch runtime.GOOS {
	case "darwin":
		return // Due to https://github.com/containers/storage/pull/811 , c/storage can be used on macOS unprivileged.
	case "linux":
		if os.Geteuid() != 0 {
			t.Skip("test requires root privileges on Linux")
		}
	default:
		// Unknown, let’s leave the tests enabled so that this can be investigated when working on that architecture.
	}
}

func createUncommittedImageDest(t *testing.T, ref types.ImageReference, cache types.BlobInfoCache,
	layers []testBlob, config *testBlob) (types.ImageDestination, types.UnparsedImage) {
	dest, err := ref.NewImageDestination(context.Background(), nil)
	require.NoError(t, err)

	layerDescriptors := []manifest.Schema2Descriptor{}
	for _, layer := range layers {
		desc := layer.storeBlob(t, dest, cache, manifest.DockerV2Schema2LayerMediaType)
		layerDescriptors = append(layerDescriptors, desc)
	}
	configDescriptor := manifest.Schema2Descriptor{} // might be good enough
	if config != nil {
		configDescriptor = config.storeBlob(t, dest, cache, manifest.DockerV2Schema2ConfigMediaType)
	}

	manifest := manifest.Schema2FromComponents(configDescriptor, layerDescriptors)
	manifestBytes, err := manifest.Serialize()
	require.NoError(t, err)
	err = dest.PutManifest(context.Background(), manifestBytes, nil)
	require.NoError(t, err)
	unparsedToplevel := unparsedImage{
		imageReference: nil,
		manifestBytes:  manifestBytes,
		manifestType:   manifest.MediaType,
		signatures:     nil,
	}
	return dest, &unparsedToplevel
}

func createImage(t *testing.T, ref types.ImageReference, cache types.BlobInfoCache,
	layers []testBlob, config *testBlob) {
	dest, unparsedToplevel := createUncommittedImageDest(t, ref, cache, layers, config)
	err := dest.Commit(context.Background(), unparsedToplevel)
	require.NoError(t, err)
	err = dest.Close()
	require.NoError(t, err)
}

func TestWriteRead(t *testing.T) {
	ensureTestCanCreateImages(t)

	configBytes := []byte(`{"config":{"labels":{}},"created":"2006-01-02T15:04:05Z"}`)
	config := testBlob{
		compressedDigest: digest.SHA256.FromBytes(configBytes),
		uncompressedSize: int64(len(configBytes)),
		compressedSize:   int64(len(configBytes)),
		data:             configBytes,
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
	cache := memory.New()

	ref, err := Transport.ParseReference("test")
	require.NoError(t, err)

	for _, manifestFmt := range manifests {
		dest, err := ref.NewImageDestination(context.Background(), nil)
		require.NoError(t, err)
		require.Equal(t, ref.StringWithinTransport(), dest.Reference().StringWithinTransport())
		t.Logf("supported manifest MIME types: %v", dest.SupportedManifestMIMETypes())
		err = dest.SupportsSignatures(context.Background())
		require.NoError(t, err)
		t.Logf("compress layers: %v", dest.DesiredLayerCompression())
		compression := archive.Uncompressed
		if dest.DesiredLayerCompression() == types.Compress {
			compression = archive.Gzip
		}
		layer := makeLayer(t, compression)
		_ = layer.storeBlob(t, dest, cache, manifest.DockerV2Schema2LayerMediaType)
		t.Logf("Wrote randomly-generated layer %q (%d/%d bytes) to destination", layer.compressedDigest, layer.compressedSize, layer.uncompressedSize)
		_ = config.storeBlob(t, dest, cache, manifest.DockerV2Schema2ConfigMediaType)

		manifest := strings.ReplaceAll(manifestFmt, "%lh", layer.compressedDigest.String())
		manifest = strings.ReplaceAll(manifest, "%ch", config.compressedDigest.String())
		manifest = strings.ReplaceAll(manifest, "%ls", fmt.Sprintf("%d", layer.compressedSize))
		manifest = strings.ReplaceAll(manifest, "%cs", fmt.Sprintf("%d", config.compressedSize))
		manifest = strings.ReplaceAll(manifest, "%li", layer.compressedDigest.Hex())
		manifest = strings.ReplaceAll(manifest, "%ci", config.compressedDigest.Hex())
		t.Logf("this manifest is %q", manifest)
		err = dest.PutManifest(context.Background(), []byte(manifest), nil)
		require.NoError(t, err)
		err = dest.PutSignatures(context.Background(), signatures, nil)
		require.NoError(t, err)
		unparsedToplevel := unparsedImage{
			imageReference: nil,
			manifestBytes:  []byte(manifest),
			manifestType:   imanifest.GuessMIMEType([]byte(manifest)),
			signatures:     signatures,
		}
		err = dest.Commit(context.Background(), &unparsedToplevel)
		require.NoError(t, err)
		err = dest.Close()
		require.NoError(t, err)

		img, err := ref.NewImage(context.Background(), nil)
		require.NoError(t, err)
		imageConfigInfo := img.ConfigInfo()
		if imageConfigInfo.Digest != "" {
			blob, err := img.ConfigBlob(context.Background())
			require.NoError(t, err)
			sum := digest.SHA256.FromBytes(blob)
			assert.Equal(t, config.compressedDigest, sum)
			assert.Len(t, blob, int(config.compressedSize))
		}
		layerInfos := img.LayerInfos()
		assert.NotNil(t, layerInfos)
		imageInfo, err := img.Inspect(context.Background())
		require.NoError(t, err)
		assert.False(t, imageInfo.Created.IsZero())

		src, err := ref.NewImageSource(context.Background(), nil)
		require.NoError(t, err)
		if src.Reference().StringWithinTransport() != ref.StringWithinTransport() {
			// As long as it's only the addition of an ID suffix, that's okay.
			assert.True(t, strings.HasPrefix(src.Reference().StringWithinTransport(), ref.StringWithinTransport()+"@"))
		}
		_, manifestType, err := src.GetManifest(context.Background(), nil)
		require.NoError(t, err)
		t.Logf("this manifest's type appears to be %q", manifestType)
		instanceDigest, err := imanifest.Digest([]byte(manifest))
		require.NoError(t, err)
		retrieved, _, err := src.GetManifest(context.Background(), &instanceDigest)
		require.NoError(t, err)
		assert.Equal(t, manifest, string(retrieved))
		sigs, err := src.GetSignatures(context.Background(), nil)
		require.NoError(t, err)
		assert.Equal(t, signatures, sigs)
		sigs2, err := src.GetSignatures(context.Background(), &instanceDigest)
		require.NoError(t, err)
		assert.Equal(t, sigs, sigs2)
		for _, layerInfo := range layerInfos {
			buf := bytes.Buffer{}
			layer, size, err := src.GetBlob(context.Background(), layerInfo, cache)
			require.NoError(t, err)
			t.Logf("Decompressing blob %q, blob size = %d, layerInfo.Size = %d bytes", layerInfo.Digest, size, layerInfo.Size)
			hasher := sha256.New()
			compressed := ioutils.NewWriteCounter(hasher)
			countedLayer := io.TeeReader(layer, compressed)
			decompressed, err := archive.DecompressStream(countedLayer)
			require.NoError(t, err)
			n, err := io.Copy(&buf, decompressed)
			require.NoError(t, err)
			layer.Close()
			if layerInfo.Size >= 0 {
				assert.Equal(t, layerInfo.Size, compressed.Count)
				assert.Equal(t, layerInfo.Size, n)
			}
			if size >= 0 {
				assert.Equal(t, size, compressed.Count)
			}
			sum := hasher.Sum(nil)
			assert.Equal(t, layerInfo.Digest, digest.NewDigestFromBytes(digest.SHA256, sum))
		}
		err = src.Close()
		require.NoError(t, err)
		err = img.Close()
		require.NoError(t, err)
		err = ref.DeleteImage(context.Background(), nil)
		require.NoError(t, err)
	}
}

func TestDuplicateName(t *testing.T) {
	ensureTestCanCreateImages(t)

	newStore(t)
	cache := memory.New()

	ref, err := Transport.ParseReference("test")
	require.NoError(t, err)

	createImage(t, ref, cache, []testBlob{makeLayer(t, archive.Uncompressed)}, nil)
	createImage(t, ref, cache, []testBlob{makeLayer(t, archive.Gzip)}, nil)
}

func TestDuplicateID(t *testing.T) {
	ensureTestCanCreateImages(t)

	newStore(t)
	cache := memory.New()

	ref, err := Transport.ParseReference("@aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	require.NoError(t, err)

	createImage(t, ref, cache, []testBlob{makeLayer(t, archive.Gzip)}, nil)

	dest, unparsedToplevel := createUncommittedImageDest(t, ref, cache,
		[]testBlob{makeLayer(t, archive.Gzip)}, nil)
	err = dest.Commit(context.Background(), unparsedToplevel)
	require.Error(t, err)
	assert.ErrorIs(t, err, storage.ErrDuplicateID)
	err = dest.Close()
	require.NoError(t, err)
}

func TestDuplicateNameID(t *testing.T) {
	ensureTestCanCreateImages(t)

	newStore(t)
	cache := memory.New()

	ref, err := Transport.ParseReference("test@aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	require.NoError(t, err)

	createImage(t, ref, cache, []testBlob{makeLayer(t, archive.Gzip)}, nil)

	dest, unparsedToplevel := createUncommittedImageDest(t, ref, cache,
		[]testBlob{makeLayer(t, archive.Gzip)}, nil)
	err = dest.Commit(context.Background(), unparsedToplevel)
	require.Error(t, err)
	assert.ErrorIs(t, err, storage.ErrDuplicateID)
	err = dest.Close()
	require.NoError(t, err)
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
	ensureTestCanCreateImages(t)

	newStore(t)
	cache := memory.New()

	layer1 := makeLayer(t, archive.Gzip)
	layer2 := makeLayer(t, archive.Gzip)
	configBytes := []byte(`{"config":{"labels":{}},"created":"2006-01-02T15:04:05Z"}`)
	config := testBlob{
		compressedDigest: digest.SHA256.FromBytes(configBytes),
		uncompressedSize: int64(len(configBytes)),
		compressedSize:   int64(len(configBytes)),
		data:             configBytes,
	}

	ref, err := Transport.ParseReference("test")
	require.NoError(t, err)

	createImage(t, ref, cache, []testBlob{layer1, layer2}, &config)

	img, err := ref.NewImage(context.Background(), nil)
	require.NoError(t, err)
	manifest, _, err := img.Manifest(context.Background())
	require.NoError(t, err)

	usize, err := img.Size()
	require.NoError(t, err)
	require.NotEqual(t, -1, usize)

	assert.Equal(t, config.compressedSize+layer1.uncompressedSize+layer2.uncompressedSize+2*int64(len(manifest)), usize)
	err = img.Close()
	require.NoError(t, err)
}

func TestDuplicateBlob(t *testing.T) {
	ensureTestCanCreateImages(t)

	newStore(t)
	cache := memory.New()

	ref, err := Transport.ParseReference("test")
	require.NoError(t, err)

	layer1 := makeLayer(t, archive.Gzip)
	layer2 := makeLayer(t, archive.Gzip)
	configBytes := []byte(`{"config":{"labels":{}},"created":"2006-01-02T15:04:05Z"}`)
	config := testBlob{
		compressedDigest: digest.SHA256.FromBytes(configBytes),
		uncompressedSize: int64(len(configBytes)),
		compressedSize:   int64(len(configBytes)),
		data:             configBytes,
	}

	createImage(t, ref, cache, []testBlob{layer1, layer2, layer1, layer2}, &config)

	img, err := ref.NewImage(context.Background(), nil)
	require.NoError(t, err)
	src, err := ref.NewImageSource(context.Background(), nil)
	require.NoError(t, err)
	source, ok := src.(*storageImageSource)
	require.True(t, ok)

	layers := []string{}
	layersInfo, err := img.LayerInfosForCopy(context.Background())
	require.NoError(t, err)
	for _, layerInfo := range layersInfo {
		digestLayers, _ := source.imageRef.transport.store.LayersByUncompressedDigest(layerInfo.Digest)
		rc, _, layerID, err := source.getBlobAndLayerID(layerInfo.Digest, digestLayers)
		require.NoError(t, err)
		_, err = io.Copy(io.Discard, rc)
		require.NoError(t, err)
		rc.Close()
		layers = append(layers, layerID)
	}
	assert.Len(t, layers, 4)
	for i, layerID := range layers {
		for j, otherID := range layers {
			if i != j && layerID == otherID {
				t.Fatalf("Layer IDs are not unique: %v", layers)
			}
		}
	}
	err = src.Close()
	require.NoError(t, err)
	err = img.Close()
	require.NoError(t, err)
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

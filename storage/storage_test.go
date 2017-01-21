package storage

import (
	"archive/tar"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/containers/image/types"
	"github.com/containers/storage/pkg/archive"
	"github.com/containers/storage/pkg/idtools"
	"github.com/containers/storage/pkg/ioutils"
	"github.com/containers/storage/pkg/reexec"
	"github.com/containers/storage/storage"
	ddigest "github.com/opencontainers/go-digest"
)

var (
	_imgd      types.ImageDestination = &storageImageDestination{}
	_imgs      types.ImageSource      = &storageImageSource{}
	_ref       types.ImageReference   = &storageReference{}
	_transport types.ImageTransport   = &storageTransport{}
	topwd                             = ""
)

const (
	layerSize = 12345
)

func TestMain(m *testing.M) {
	if reexec.Init() {
		return
	}
	wd, err := ioutil.TempDir("", "test.")
	if err != nil {
		os.Exit(1)
	}
	topwd = wd
	debug := false
	flag.BoolVar(&debug, "debug", false, "print debug statements")
	flag.Parse()
	if debug {
		logrus.SetLevel(logrus.DebugLevel)
	}
	code := m.Run()
	os.RemoveAll(wd)
	os.Exit(code)
}

func newStore(t *testing.T) storage.Store {
	wd, err := ioutil.TempDir(topwd, "test.")
	if err != nil {
		t.Fatal(err)
	}
	err = os.MkdirAll(wd, 0700)
	if err != nil {
		t.Fatal(err)
	}
	run := filepath.Join(wd, "run")
	root := filepath.Join(wd, "root")
	uidmap := []idtools.IDMap{{
		ContainerID: 0,
		HostID:      os.Getuid(),
		Size:        1,
	}}
	gidmap := []idtools.IDMap{{
		ContainerID: 0,
		HostID:      os.Getgid(),
		Size:        1,
	}}
	store, err := storage.GetStore(storage.StoreOptions{
		RunRoot:            run,
		GraphRoot:          root,
		GraphDriverName:    "vfs",
		GraphDriverOptions: []string{},
		UidMap:             uidmap,
		GidMap:             gidmap,
	})
	if err != nil {
		t.Fatal(err)
	}
	Transport.SetStore(store)
	return store
}

func TestParse(t *testing.T) {
	store := newStore(t)
	ref := getRef("test", t)

	var err error
	ref, err = Transport.ParseStoreReference(store, "test")
	if err != nil {
		t.Fatalf("ParseStoreReference(%q) returned error %v", "test", err)
	}
	if ref == nil {
		t.Fatalf("ParseStoreReference(%q) returned nil reference", "test")
	}

	strRef := ref.StringWithinTransport()
	ref = getRef(strRef, t)

	transport := storageTransport{
		store: store,
	}
	_references := []storageReference{
		{
			name:      ref.(*storageReference).name,
			reference: verboseName(ref.(*storageReference).name),
			id:        "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			transport: transport,
		},
		{
			name:      ref.(*storageReference).name,
			reference: verboseName(ref.(*storageReference).name),
			transport: transport,
		},
		{
			id:        "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			transport: transport,
		},
		{
			name:      ref.DockerReference(),
			reference: verboseName(ref.DockerReference()),
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
		if ref.reference != reference.reference {
			t.Fatalf("ParseReference(%q) failed to extract reference (%q!=%q)", s, ref.reference, reference.reference)
		}
	}
}

func systemContext() *types.SystemContext {
	return &types.SystemContext{}
}

func makeLayer(t *testing.T, compression archive.Compression) (ddigest.Digest, int64, int64, []byte) {
	var cwriter io.WriteCloser
	var uncompressed *ioutils.WriteCounter
	var twriter *tar.Writer
	preader, pwriter := io.Pipe()
	tbuffer := bytes.Buffer{}
	if compression != archive.Uncompressed {
		compressor, err := archive.CompressStream(pwriter, compression)
		if err != nil {
			t.Fatalf("Error compressing layer: %v", err)
		}
		cwriter = compressor
		uncompressed = ioutils.NewWriteCounter(cwriter)
	} else {
		uncompressed = ioutils.NewWriteCounter(pwriter)
	}
	twriter = tar.NewWriter(uncompressed)
	buf := make([]byte, layerSize)
	n, err := rand.Read(buf)
	if err != nil {
		t.Fatalf("Error reading tar data: %v", err)
	}
	if n != len(buf) {
		t.Fatalf("Short read reading tar data: %d < %d", n, len(buf))
	}
	for i := 1024; i < 2048; i++ {
		buf[i] = 0
	}
	go func() {
		defer pwriter.Close()
		if cwriter != nil {
			defer cwriter.Close()
		}
		defer twriter.Close()
		err := twriter.WriteHeader(&tar.Header{
			Name:       "/random-single-file",
			Mode:       0600,
			Size:       int64(len(buf)),
			ModTime:    time.Now(),
			AccessTime: time.Now(),
			ChangeTime: time.Now(),
			Typeflag:   tar.TypeReg,
		})
		if err != nil {
			t.Fatalf("Error writing tar header: %v", err)
		}
		n, err := twriter.Write(buf)
		if err != nil {
			t.Fatalf("Error writing tar header: %v", err)
		}
		if n != len(buf) {
			t.Fatalf("Short write writing tar header: %d < %d", n, len(buf))
		}
	}()
	_, err = io.Copy(&tbuffer, preader)
	if err != nil {
		t.Fatalf("Error reading layer tar: %v", err)
	}
	sum := ddigest.SHA256.FromBytes(tbuffer.Bytes())
	return sum, uncompressed.Count, int64(tbuffer.Len()), tbuffer.Bytes()
}

func requireRoot(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("This test requires root privileges")
	}
}

type imageDestinationParameters struct {
	ref        types.ImageReference
	manifest   string
	configInfo types.BlobInfo
	digest     ddigest.Digest
	size       int64
	sum        ddigest.Digest
	dest       types.ImageDestination
	config     string
	signatures [][]byte
	layerInfos []types.BlobInfo
	src        types.ImageSource
}

func buildManifest(params *imageDestinationParameters) string {
	manifest := strings.Replace(params.manifest, "%lh", params.digest.String(), -1)
	manifest = strings.Replace(manifest, "%ch", params.configInfo.Digest.String(), -1)
	manifest = strings.Replace(manifest, "%ls", fmt.Sprintf("%d", params.size), -1)
	manifest = strings.Replace(manifest, "%cs", fmt.Sprintf("%d", params.configInfo.Size), -1)
	li := params.digest.Hex()
	manifest = strings.Replace(manifest, "%li", li, -1)
	manifest = strings.Replace(manifest, "%ci", params.sum.Hex(), -1)

	return manifest
}

func testDestinationConstructor(params *imageDestinationParameters, t *testing.T) {
	if params.dest == nil {
		t.Fatalf("NewImageDestination(%q) returned no destination", params.ref.StringWithinTransport())
	}
	if params.dest.Reference().StringWithinTransport() != params.ref.StringWithinTransport() {
		t.Fatalf("NewImageDestination(%q) changed the reference to %q", params.ref.StringWithinTransport(), params.dest.Reference().StringWithinTransport())
	}
	t.Logf("supported manifest MIME types: %v", params.dest.SupportedManifestMIMETypes())
	if err := params.dest.SupportsSignatures(); err != nil {
		t.Fatalf("Destination image doesn't support signatures: %v", err)
	}
}

func testImageDestination(params *imageDestinationParameters, t *testing.T) {
	dest, err := params.ref.NewImageDestination(systemContext())
	if err != nil {
		t.Fatalf("NewImageDestination(%q) returned error %v", params.ref.StringWithinTransport(), err)
	}

	params.dest = dest

	testDestinationConstructor(params, t)

	t.Logf("compress layers: %v", dest.ShouldCompressLayers())
	compression := archive.Uncompressed
	if dest.ShouldCompressLayers() {
		compression = archive.Gzip
	}
	digest, decompressedSize, size, blob := makeLayer(t, compression)
	if _, err := dest.PutBlob(bytes.NewBuffer(blob), types.BlobInfo{
		Size:   size,
		Digest: digest,
	}); err != nil {
		t.Fatalf("Error saving randomly-generated layer to destination: %v", err)
	}
	t.Logf("Wrote randomly-generated layer %q (%d/%d bytes) to destination", digest, size, decompressedSize)
	if _, err := dest.PutBlob(bytes.NewBufferString(params.config), params.configInfo); err != nil {
		t.Fatalf("Error saving config to destination: %v", err)
	}

	params.size = size
	params.digest = digest

	manifest := buildManifest(params)

	if err := dest.PutManifest([]byte(manifest)); err != nil {
		t.Fatalf("Error saving manifest to destination: %v", err)
	}
	if err := dest.PutSignatures(params.signatures); err != nil {
		t.Fatalf("Error saving signatures to destination: %v", err)
	}
	if err := dest.Commit(); err != nil {
		t.Fatalf("Error committing changes to destination: %v", err)
	}
	dest.Close()
}

func testNewImage(params *imageDestinationParameters, t *testing.T) {
	img, err := params.ref.NewImage(systemContext())
	defer img.Close()
	if err != nil {
		t.Fatalf("NewImage(%q) returned error %v", params.ref.StringWithinTransport(), err)
	}
	imageConfigInfo := img.ConfigInfo()
	if imageConfigInfo.Digest != "" {
		blob, err := img.ConfigBlob()
		if err != nil {
			t.Fatalf("image %q claimed there was a config blob, but couldn't produce it: %v", params.ref.StringWithinTransport(), err)
		}
		sum := ddigest.SHA256.FromBytes(blob)
		if sum != params.configInfo.Digest {
			t.Fatalf("image config blob digest for %q doesn't match", params.ref.StringWithinTransport())
		}
		if int64(len(blob)) != params.configInfo.Size {
			t.Fatalf("image config size for %q changed from %d to %d", params.ref.StringWithinTransport(), params.configInfo.Size, len(blob))
		}
	}
	params.layerInfos = img.LayerInfos()
	if params.layerInfos == nil {
		t.Fatalf("image for %q returned empty layer list", params.ref.StringWithinTransport())
	}
	imageInfo, err := img.Inspect()
	if err != nil {
		t.Fatalf("Inspect(%q) returned error %v", params.ref.StringWithinTransport(), err)
	}
	if imageInfo.Created.IsZero() {
		t.Fatalf("Image %q claims to have been created at time 0", params.ref.StringWithinTransport())
	}
}

func testImageSource(params *imageDestinationParameters, t *testing.T) {
	src, err := params.ref.NewImageSource(systemContext(), []string{})
	if err != nil {
		t.Fatalf("NewImageSource(%q) returned error %v", params.ref.StringWithinTransport(), err)
	}

	if src == nil {
		t.Fatalf("NewImageSource(%q) returned no source", params.ref.StringWithinTransport())
	}

	params.src = src

	if src.Reference().StringWithinTransport() != params.ref.StringWithinTransport() {
		// As long as it's only the addition of an ID suffix, that's okay.
		if !strings.HasPrefix(src.Reference().StringWithinTransport(), params.ref.StringWithinTransport()+"@") {
			t.Fatalf("NewImageSource(%q) changed the reference to %q", params.ref.StringWithinTransport(), src.Reference().StringWithinTransport())
		}
	}
	retrievedManifest, manifestType, err := src.GetManifest()
	if err != nil {
		t.Fatalf("GetManifest(%q) returned error %v", params.ref.StringWithinTransport(), err)
	}
	t.Logf("this manifest's type appears to be %q", manifestType)

	manifest := buildManifest(params)
	if string(retrievedManifest) != manifest {
		t.Fatalf("NewImageSource(%q) changed the manifest: %q was %q", params.ref.StringWithinTransport(), string(retrievedManifest), manifest)
	}
	params.sum = ddigest.SHA256.FromBytes([]byte(manifest))
	_, _, err = src.GetTargetManifest(params.sum)
	if err == nil {
		t.Fatalf("GetTargetManifest(%q) is supposed to fail", params.ref.StringWithinTransport())
	}
	sigs, err := src.GetSignatures()
	if err != nil {
		t.Fatalf("GetSignatures(%q) returned error %v", params.ref.StringWithinTransport(), err)
	}
	if len(sigs) < len(params.signatures) {
		t.Fatalf("Lost %d signatures", len(params.signatures)-len(sigs))
	}
	if len(sigs) > len(params.signatures) {
		t.Fatalf("Gained %d signatures", len(sigs)-len(params.signatures))
	}
	for i := range sigs {
		if bytes.Compare(sigs[i], params.signatures[i]) != 0 {
			t.Fatalf("Signature %d was corrupted", i)
		}
	}

	testLayerInfos(params, t)

	src.Close()
	err = params.ref.DeleteImage(systemContext())
	if err != nil {
		t.Fatalf("DeleteImage(%q) returned error %v", params.ref.StringWithinTransport(), err)
	}
}

func testLayerInfos(params *imageDestinationParameters, t *testing.T) {
	for _, layerInfo := range params.layerInfos {
		buf := bytes.Buffer{}
		layer, size, err := params.src.GetBlob(layerInfo)
		if err != nil {
			t.Fatalf("Error reading layer %q from %q", layerInfo.Digest, params.ref.StringWithinTransport())
		}
		t.Logf("Decompressing blob %q, blob size = %d, layerInfo.Size = %d bytes", layerInfo.Digest, size, layerInfo.Size)
		hasher := sha256.New()
		compressed := ioutils.NewWriteCounter(hasher)
		countedLayer := io.TeeReader(layer, compressed)
		decompressed, err := archive.DecompressStream(countedLayer)
		if err != nil {
			t.Fatalf("Error decompressing layer %q from %q", layerInfo.Digest, params.ref.StringWithinTransport())
		}
		n, err := io.Copy(&buf, decompressed)
		if layerInfo.Size >= 0 && compressed.Count != layerInfo.Size {
			t.Fatalf("Blob size is different than expected: %d != %d, read %d", compressed.Count, layerInfo.Size, n)
		}
		if size >= 0 && compressed.Count != size {
			t.Fatalf("Blob size mismatch: %d != %d, read %d", compressed.Count, size, n)
		}
		sum := hasher.Sum(nil)
		if ddigest.NewDigestFromBytes(ddigest.SHA256, sum) != layerInfo.Digest {
			t.Fatalf("Layer blob digest for %q doesn't match", params.ref.StringWithinTransport())
		}
	}
}

func TestWriteRead(t *testing.T) {
	requireRoot(t)

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
	signatures := [][]byte{
		[]byte("Signature A"),
		[]byte("Signature B"),
	}

	newStore(t)
	ref := getRef("test", t)

	for _, manifestFmt := range manifests {
		params := &imageDestinationParameters{
			ref:        ref,
			manifest:   manifestFmt,
			configInfo: configInfo,
			sum:        sum,
			config:     config,
			signatures: signatures,
		}

		testImageDestination(params, t)
		testNewImage(params, t)
		testImageSource(params, t)
	}
}

func TestDuplicateName(t *testing.T) {
	requireRoot(t)
	newStore(t)

	ref, err := Transport.ParseReference("test")
	if err != nil {
		t.Fatalf("ParseReference(%q) returned error %v", "test", err)
	}
	if ref == nil {
		t.Fatalf("ParseReference returned nil reference")
	}

	dest, err := ref.NewImageDestination(systemContext())
	if err != nil {
		t.Fatalf("NewImageDestination(%q, first pass) returned error %v", ref.StringWithinTransport(), err)
	}
	if dest == nil {
		t.Fatalf("NewImageDestination(%q, first pass) returned no destination", ref.StringWithinTransport())
	}
	digest, _, size, blob := makeLayer(t, archive.Uncompressed)
	if _, err := dest.PutBlob(bytes.NewBuffer(blob), types.BlobInfo{
		Size:   size,
		Digest: digest,
	}); err != nil {
		t.Fatalf("Error saving randomly-generated layer to destination, first pass: %v", err)
	}
	if err := dest.Commit(); err != nil {
		t.Fatalf("Error committing changes to destination, first pass: %v", err)
	}
	dest.Close()

	dest, err = ref.NewImageDestination(systemContext())
	if err != nil {
		t.Fatalf("NewImageDestination(%q, second pass) returned error %v", ref.StringWithinTransport(), err)
	}
	if dest == nil {
		t.Fatalf("NewImageDestination(%q, second pass) returned no destination", ref.StringWithinTransport())
	}
	if _, err := dest.PutBlob(bytes.NewBuffer(blob), types.BlobInfo{
		Size:   int64(size),
		Digest: digest,
	}); err != nil {
		t.Fatalf("Error saving randomly-generated layer to destination, second pass: %v", err)
	}
	if err := dest.Commit(); err != nil {
		t.Fatalf("Error committing changes to destination, second pass: %v", err)
	}
	dest.Close()
}

func TestDuplicateID(t *testing.T) {
	requireRoot(t)
	newStore(t)

	ref, err := Transport.ParseReference("@aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatalf("ParseReference(%q) returned error %v", "test", err)
	}
	if ref == nil {
		t.Fatalf("ParseReference returned nil reference")
	}

	dest, err := ref.NewImageDestination(systemContext())
	if err != nil {
		t.Fatalf("NewImageDestination(%q, first pass) returned error %v", ref.StringWithinTransport(), err)
	}
	if dest == nil {
		t.Fatalf("NewImageDestination(%q, first pass) returned no destination", ref.StringWithinTransport())
	}
	digest, _, size, blob := makeLayer(t, archive.Gzip)
	if _, err := dest.PutBlob(bytes.NewBuffer(blob), types.BlobInfo{
		Size:   size,
		Digest: digest,
	}); err != nil {
		t.Fatalf("Error saving randomly-generated layer to destination, first pass: %v", err)
	}
	if err := dest.Commit(); err != nil {
		t.Fatalf("Error committing changes to destination, first pass: %v", err)
	}
	dest.Close()

	dest, err = ref.NewImageDestination(systemContext())
	if err != nil {
		t.Fatalf("NewImageDestination(%q, second pass) returned error %v", ref.StringWithinTransport(), err)
	}
	if dest == nil {
		t.Fatalf("NewImageDestination(%q, second pass) returned no destination", ref.StringWithinTransport())
	}
	if _, err := dest.PutBlob(bytes.NewBuffer(blob), types.BlobInfo{
		Size:   int64(size),
		Digest: digest,
	}); err != nil {
		t.Fatalf("Error saving randomly-generated layer to destination, second pass: %v", err)
	}
	if err := dest.Commit(); err != storage.ErrDuplicateID {
		if err != nil {
			t.Fatalf("Wrong error committing changes to destination, second pass: %v", err)
		}
		t.Fatalf("Incorrectly succeeded committing changes to destination, second pass: %v", err)
	}
	dest.Close()
}

func TestDuplicateNameID(t *testing.T) {
	requireRoot(t)
	newStore(t)

	ref, err := Transport.ParseReference("test@aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatalf("ParseReference(%q) returned error %v", "test", err)
	}
	if ref == nil {
		t.Fatalf("ParseReference returned nil reference")
	}

	dest, err := ref.NewImageDestination(systemContext())
	if err != nil {
		t.Fatalf("NewImageDestination(%q, first pass) returned error %v", ref.StringWithinTransport(), err)
	}
	if dest == nil {
		t.Fatalf("NewImageDestination(%q, first pass) returned no destination", ref.StringWithinTransport())
	}
	digest, _, size, blob := makeLayer(t, archive.Gzip)
	if _, err := dest.PutBlob(bytes.NewBuffer(blob), types.BlobInfo{
		Size:   size,
		Digest: digest,
	}); err != nil {
		t.Fatalf("Error saving randomly-generated layer to destination, first pass: %v", err)
	}
	if err := dest.Commit(); err != nil {
		t.Fatalf("Error committing changes to destination, first pass: %v", err)
	}
	dest.Close()

	dest, err = ref.NewImageDestination(systemContext())
	if err != nil {
		t.Fatalf("NewImageDestination(%q, second pass) returned error %v", ref.StringWithinTransport(), err)
	}
	if dest == nil {
		t.Fatalf("NewImageDestination(%q, second pass) returned no destination", ref.StringWithinTransport())
	}
	if _, err := dest.PutBlob(bytes.NewBuffer(blob), types.BlobInfo{
		Size:   int64(size),
		Digest: digest,
	}); err != nil {
		t.Fatalf("Error saving randomly-generated layer to destination, second pass: %v", err)
	}
	if err := dest.Commit(); err != storage.ErrDuplicateID {
		if err != nil {
			t.Fatalf("Wrong error committing changes to destination, second pass: %v", err)
		}
		t.Fatalf("Incorrectly succeeded committing changes to destination, second pass: %v", err)
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
	requireRoot(t)

	config := `{"config":{"labels":{}},"created":"2006-01-02T15:04:05Z"}`
	sum := ddigest.SHA256.FromBytes([]byte(config))
	configInfo := types.BlobInfo{
		Digest: sum,
		Size:   int64(len(config)),
	}

	newStore(t)

	ref, err := Transport.ParseReference("test")
	if err != nil {
		t.Fatalf("ParseReference(%q) returned error %v", "test", err)
	}
	if ref == nil {
		t.Fatalf("ParseReference returned nil reference")
	}

	dest, err := ref.NewImageDestination(systemContext())
	if err != nil {
		t.Fatalf("NewImageDestination(%q) returned error %v", ref.StringWithinTransport(), err)
	}
	if dest == nil {
		t.Fatalf("NewImageDestination(%q) returned no destination", ref.StringWithinTransport())
	}
	digest1, _, size1, blob := makeLayer(t, archive.Gzip)
	if _, err := dest.PutBlob(bytes.NewBuffer(blob), types.BlobInfo{
		Size:   size1,
		Digest: digest1,
	}); err != nil {
		t.Fatalf("Error saving randomly-generated layer 1 to destination: %v", err)
	}
	digest2, _, size2, blob := makeLayer(t, archive.Gzip)
	if _, err := dest.PutBlob(bytes.NewBuffer(blob), types.BlobInfo{
		Size:   size2,
		Digest: digest2,
	}); err != nil {
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
	if err := dest.PutManifest([]byte(manifest)); err != nil {
		t.Fatalf("Error storing manifest to destination: %v", err)
	}
	if err := dest.Commit(); err != nil {
		t.Fatalf("Error committing changes to destination: %v", err)
	}
	dest.Close()

	img, err := ref.NewImage(systemContext())
	if err != nil {
		t.Fatalf("NewImage(%q) returned error %v", ref.StringWithinTransport(), err)
	}
	usize, err := img.Size()
	if usize == -1 || err != nil {
		t.Fatalf("Error calculating image size: %v", err)
	}
	if int(usize) != layerSize*2+len(manifest) {
		t.Fatalf("Unexpected image size: %d != %d + %d + %d", usize, layerSize, layerSize, len(manifest))
	}
	img.Close()
}

func getRef(name string, t *testing.T) types.ImageReference {
	ref, err := Transport.ParseReference(name)
	if err != nil {
		t.Fatalf("ParseReference(%q) returned error %v", name, err)
	}
	if ref == nil {
		t.Fatalf("ParseReference returned nil reference")
	}

	return ref
}

func testDestinationBlob(ref types.ImageReference, configInfo types.BlobInfo, t *testing.T) {
	dest, err := ref.NewImageDestination(systemContext())
	if err != nil {
		t.Fatalf("NewImageDestination(%q) returned error %v", ref.StringWithinTransport(), err)
	}
	if dest == nil {
		t.Fatalf("NewImageDestination(%q) returned no destination", ref.StringWithinTransport())
	}
	digest1, _, size1, blob1 := makeLayer(t, archive.Gzip)
	if _, err := dest.PutBlob(bytes.NewBuffer(blob1), types.BlobInfo{
		Size:   size1,
		Digest: digest1,
	}); err != nil {
		t.Fatalf("Error saving randomly-generated layer 1 to destination (first copy): %v", err)
	}
	digest2, _, size2, blob2 := makeLayer(t, archive.Gzip)
	if _, err := dest.PutBlob(bytes.NewBuffer(blob2), types.BlobInfo{
		Size:   size2,
		Digest: digest2,
	}); err != nil {
		t.Fatalf("Error saving randomly-generated layer 2 to destination (first copy): %v", err)
	}
	if _, err := dest.PutBlob(bytes.NewBuffer(blob1), types.BlobInfo{
		Size:   size1,
		Digest: digest1,
	}); err != nil {
		t.Fatalf("Error saving randomly-generated layer 1 to destination (second copy): %v", err)
	}
	if _, err := dest.PutBlob(bytes.NewBuffer(blob2), types.BlobInfo{
		Size:   size2,
		Digest: digest2,
	}); err != nil {
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
	if err := dest.PutManifest([]byte(manifest)); err != nil {
		t.Fatalf("Error storing manifest to destination: %v", err)
	}
	if err := dest.Commit(); err != nil {
		t.Fatalf("Error committing changes to destination: %v", err)
	}
	dest.Close()
}

func TestDuplicateBlob(t *testing.T) {
	requireRoot(t)

	config := `{"config":{"labels":{}},"created":"2006-01-02T15:04:05Z"}`
	sum := ddigest.SHA256.FromBytes([]byte(config))
	configInfo := types.BlobInfo{
		Digest: sum,
		Size:   int64(len(config)),
	}

	ref := getRef("test", t)

	newStore(t)
	testDestinationBlob(ref, configInfo, t)

	img, err := ref.NewImage(systemContext())
	if err != nil {
		t.Fatalf("NewImage(%q) returned error %v", ref.StringWithinTransport(), err)
	}
	src, err := ref.NewImageSource(systemContext(), nil)
	if err != nil {
		t.Fatalf("NewImageSource(%q) returned error %v", ref.StringWithinTransport(), err)
	}
	source, ok := src.(*storageImageSource)
	if !ok {
		t.Fatalf("ImageSource is not a storage image")
	}
	layers := []string{}
	for _, layerInfo := range img.LayerInfos() {
		rc, _, layerID, err := source.getBlobAndLayerID(layerInfo)
		if err != nil {
			t.Fatalf("getBlobAndLayerID(%q) returned error %v", layerInfo.Digest, err)
		}
		io.Copy(ioutil.Discard, rc)
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

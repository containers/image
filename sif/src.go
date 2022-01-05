package sifimage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"time"

	"github.com/containers/image/v5/internal/tmpdir"
	"github.com/containers/image/v5/types"
	"github.com/klauspost/pgzip"
	"github.com/opencontainers/go-digest"
	"github.com/pkg/errors"

	imgspecs "github.com/opencontainers/image-spec/specs-go"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

type sifImageSource struct {
	ref        sifReference
	sifimg     loadedSifImage
	workdir    string
	diffID     digest.Digest
	diffSize   int64
	blobID     digest.Digest
	blobSize   int64
	blobTime   time.Time
	blobType   string
	blobFile   string
	config     []byte
	configID   digest.Digest
	configSize int64
	manifest   []byte
}

func (s *sifImageSource) getLayerInfo(tarpath string) error {
	ftar, err := os.Open(tarpath)
	if err != nil {
		return fmt.Errorf("error opening %q for reading: %v", tarpath, err)
	}
	defer ftar.Close()

	diffDigester := digest.Canonical.Digester()
	s.diffSize, err = io.Copy(diffDigester.Hash(), ftar)
	if err != nil {
		return fmt.Errorf("error reading %q: %v", tarpath, err)
	}
	s.diffID = diffDigester.Digest()

	return nil
}
func (s *sifImageSource) createBlob(tarpath string) error {
	s.blobFile = fmt.Sprintf("%s.%s", tarpath, "gz")
	fgz, err := os.Create(s.blobFile)
	if err != nil {
		return errors.Wrapf(err, "creating file for compressed blob")
	}
	defer fgz.Close()
	fileinfo, err := fgz.Stat()
	if err != nil {
		return fmt.Errorf("error reading modtime of %q: %v", s.blobFile, err)
	}
	s.blobTime = fileinfo.ModTime()

	ftar, err := os.Open(tarpath)
	if err != nil {
		return fmt.Errorf("error opening %q for reading: %v", tarpath, err)
	}
	defer ftar.Close()

	writer := pgzip.NewWriter(fgz)
	defer writer.Close()
	_, err = io.Copy(writer, ftar)
	if err != nil {
		return fmt.Errorf("error compressing %q: %v", tarpath, err)
	}

	return nil
}

func (s *sifImageSource) getBlobInfo() error {
	fgz, err := os.Open(s.blobFile)
	if err != nil {
		return fmt.Errorf("error opening %q for reading: %v", s.blobFile, err)
	}
	defer fgz.Close()

	blobDigester := digest.Canonical.Digester()
	s.blobSize, err = io.Copy(blobDigester.Hash(), fgz)
	if err != nil {
		return fmt.Errorf("error reading %q: %v", s.blobFile, err)
	}
	s.blobID = blobDigester.Digest()

	return nil
}

// newImageSource returns an ImageSource for reading from an existing directory.
// newImageSource extracts SIF objects and saves them in a temp directory.
func newImageSource(ctx context.Context, sys *types.SystemContext, ref sifReference) (types.ImageSource, error) {
	var imgSrc sifImageSource

	sifimg, err := loadSIFImage(ref.resolvedFile)
	if err != nil {
		return nil, errors.Wrap(err, "loading SIF file")
	}

	workdir, err := ioutil.TempDir(tmpdir.TemporaryDirectoryForBigFiles(sys), "sif")
	if err != nil {
		return nil, errors.Wrapf(err, "creating temp directory")
	}

	tarpath, err := sifimg.SquashFSToTarLayer(workdir)
	if err != nil {
		return nil, errors.Wrapf(err, "converting rootfs from SquashFS to Tarball")
	}

	// generate layer info
	err = imgSrc.getLayerInfo(tarpath)
	if err != nil {
		return nil, errors.Wrapf(err, "gathering layer diff information")
	}

	// prepare compressed blob
	err = imgSrc.createBlob(tarpath)
	if err != nil {
		return nil, errors.Wrapf(err, "creating blob file")
	}

	// generate blob info
	err = imgSrc.getBlobInfo()
	if err != nil {
		return nil, errors.Wrapf(err, "gathering blob information")
	}

	// populate the rootfs section of the config
	rootfs := imgspecv1.RootFS{
		Type:    "layers",
		DiffIDs: []digest.Digest{imgSrc.diffID},
	}
	created := imgSrc.blobTime
	history := []imgspecv1.History{
		{
			Created:   &created,
			CreatedBy: fmt.Sprintf("/bin/sh -c #(nop) ADD file:%s in %c", imgSrc.diffID.Hex(), os.PathSeparator),
			Comment:   "imported from SIF, uuid: " + sifimg.GetSIFID(),
		},
		{
			Created:    &created,
			CreatedBy:  "/bin/sh -c #(nop) CMD [\"bash\"]",
			EmptyLayer: true,
		},
	}

	// build an OCI image config
	var config imgspecv1.Image
	config.Created = &created
	config.Architecture = sifimg.GetSIFArch()
	config.OS = "linux"
	config.RootFS = rootfs
	config.History = history
	err = sifimg.GetConfig(&config)
	if err != nil {
		return nil, errors.Wrapf(err, "getting config elements from SIF")
	}

	// Encode and digest the image configuration blob.
	configBytes, err := json.Marshal(&config)
	if err != nil {
		return nil, fmt.Errorf("error generating configuration blob for %q: %v", ref.resolvedFile, err)
	}
	configID := digest.Canonical.FromBytes(configBytes)
	configSize := int64(len(configBytes))

	// Populate a manifest with the configuration blob and the SquashFS part as the single layer.
	layerDescriptor := imgspecv1.Descriptor{
		Digest:    imgSrc.blobID,
		Size:      imgSrc.blobSize,
		MediaType: imgspecv1.MediaTypeImageLayerGzip,
	}
	manifest := imgspecv1.Manifest{
		Versioned: imgspecs.Versioned{
			SchemaVersion: 2,
		},
		Config: imgspecv1.Descriptor{
			Digest:    configID,
			Size:      configSize,
			MediaType: imgspecv1.MediaTypeImageConfig,
		},
		Layers: []imgspecv1.Descriptor{layerDescriptor},
	}
	manifestBytes, err := json.Marshal(&manifest)
	if err != nil {
		return nil, fmt.Errorf("error generating manifest for %q: %v", ref.resolvedFile, err)
	}

	return &sifImageSource{
		ref:        ref,
		sifimg:     sifimg,
		workdir:    workdir,
		diffID:     imgSrc.diffID,
		diffSize:   imgSrc.diffSize,
		blobID:     imgSrc.blobID,
		blobSize:   imgSrc.blobSize,
		blobType:   layerDescriptor.MediaType,
		blobFile:   imgSrc.blobFile,
		config:     configBytes,
		configID:   configID,
		configSize: configSize,
		manifest:   manifestBytes,
	}, nil
}

// Reference returns the reference used to set up this source.
func (s *sifImageSource) Reference() types.ImageReference {
	return s.ref
}

// Close removes resources associated with an initialized ImageSource, if any.
func (s *sifImageSource) Close() error {
	os.RemoveAll(s.workdir)
	return s.sifimg.UnloadSIFImage()
}

// HasThreadSafeGetBlob indicates whether GetBlob can be executed concurrently.
func (s *sifImageSource) HasThreadSafeGetBlob() bool {
	return false
}

// GetBlob returns a stream for the specified blob, and the blob’s size (or -1 if unknown).
// The Digest field in BlobInfo is guaranteed to be provided, Size may be -1 and MediaType may be optionally provided.
// May update BlobInfoCache, preferably after it knows for certain that a blob truly exists at a specific location.
func (s *sifImageSource) GetBlob(ctx context.Context, info types.BlobInfo, cache types.BlobInfoCache) (io.ReadCloser, int64, error) {
	// We should only be asked about things in the manifest.  Maybe the configuration blob.
	if info.Digest == s.configID {
		return ioutil.NopCloser(bytes.NewBuffer(s.config)), s.configSize, nil
	}
	if info.Digest == s.blobID {
		reader, err := os.Open(s.blobFile)
		if err != nil {
			return nil, -1, fmt.Errorf("error opening %q: %v", s.blobFile, err)
		}
		return reader, s.blobSize, nil
	}
	return nil, -1, fmt.Errorf("no blob with digest %q found", info.Digest.String())
}

// GetManifest returns the image's manifest along with its MIME type (which may be empty when it can't be determined but the manifest is available).
// It may use a remote (= slow) service.
// If instanceDigest is not nil, it contains a digest of the specific manifest instance to retrieve (when the primary manifest is a manifest list);
// this never happens if the primary manifest is not a manifest list (e.g. if the source never returns manifest lists).
func (s *sifImageSource) GetManifest(ctx context.Context, instanceDigest *digest.Digest) ([]byte, string, error) {
	if instanceDigest != nil {
		return nil, "", fmt.Errorf("manifest lists are not supported by the sif transport")
	}
	return s.manifest, imgspecv1.MediaTypeImageManifest, nil
}

// GetSignatures returns the image's signatures.  It may use a remote (= slow) service.
// If instanceDigest is not nil, it contains a digest of the specific manifest instance to retrieve signatures for
// (when the primary manifest is a manifest list); this never happens if the primary manifest is not a manifest list
// (e.g. if the source never returns manifest lists).
func (s *sifImageSource) GetSignatures(ctx context.Context, instanceDigest *digest.Digest) ([][]byte, error) {
	if instanceDigest != nil {
		return nil, fmt.Errorf("manifest lists are not supported by the sif transport")
	}
	return nil, nil
}

// LayerInfosForCopy returns either nil (meaning the values in the manifest are fine), or updated values for the layer
// blobsums that are listed in the image's manifest.  If values are returned, they should be used when using GetBlob()
// to read the image's layers.
// If instanceDigest is not nil, it contains a digest of the specific manifest instance to retrieve BlobInfos for
// (when the primary manifest is a manifest list); this never happens if the primary manifest is not a manifest list
// (e.g. if the source never returns manifest lists).
// The Digest field is guaranteed to be provided; Size may be -1.
// WARNING: The list may contain duplicates, and they are semantically relevant.
func (s *sifImageSource) LayerInfosForCopy(ctx context.Context, instanceDigest *digest.Digest) ([]types.BlobInfo, error) {
	return nil, nil
}

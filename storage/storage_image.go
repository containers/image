package storage

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"time"

	"github.com/pkg/errors"

	"github.com/Sirupsen/logrus"
	"github.com/containers/image/image"
	"github.com/containers/image/manifest"
	"github.com/containers/image/types"
	"github.com/containers/storage/pkg/archive"
	"github.com/containers/storage/pkg/ioutils"
	"github.com/containers/storage/storage"
	ddigest "github.com/opencontainers/go-digest"
)

var (
	// ErrBlobDigestMismatch is returned when PutBlob() is given a blob
	// with a digest-based name that doesn't match its contents.
	ErrBlobDigestMismatch = errors.New("blob digest mismatch")
	// ErrBlobSizeMismatch is returned when PutBlob() is given a blob
	// with an expected size that doesn't match the reader.
	ErrBlobSizeMismatch = errors.New("blob size mismatch")
	// ErrNoManifestLists is returned when GetTargetManifest() is
	// called.
	ErrNoManifestLists = errors.New("manifest lists are not supported by this transport")
	// ErrNoSuchImage is returned when we attempt to access an image which
	// doesn't exist in the storage area.
	ErrNoSuchImage = storage.ErrNotAnImage
)

var errorBlobInfo = types.BlobInfo{
	Digest: "",
	Size:   -1,
}

type storageImageSource struct {
	imageRef       storageReference
	Tag            string                      `json:"tag,omitempty"`
	Created        time.Time                   `json:"created-time,omitempty"`
	ID             string                      `json:"id"`
	BlobList       []types.BlobInfo            `json:"blob-list,omitempty"` // Ordered list of every blob the image has been told to handle
	Layers         map[ddigest.Digest][]string `json:"layers,omitempty"`    // Map from digests of blobs to lists of layer IDs
	LayerPosition  map[ddigest.Digest]int      `json:"-"`                   // Where we are in reading a blob's layers
	SignatureSizes []int                       `json:"signature-sizes"`     // List of sizes of each signature slice
}

type storageImageDestination struct {
	imageRef       storageReference
	Tag            string                      `json:"tag,omitempty"`
	Created        time.Time                   `json:"created-time,omitempty"`
	ID             string                      `json:"id"`
	BlobList       []types.BlobInfo            `json:"blob-list,omitempty"` // Ordered list of every blob the image has been told to handle
	Layers         map[ddigest.Digest][]string `json:"layers,omitempty"`    // Map from digests of blobs to lists of layer IDs
	BlobData       map[ddigest.Digest][]byte   `json:"-"`                   // Map from names of blobs that aren't layers to contents, temporary
	Manifest       []byte                      `json:"-"`                   // Manifest contents, temporary
	Signatures     []byte                      `json:"-"`                   // Signature contents, temporary
	SignatureSizes []int                       `json:"signature-sizes"`     // List of sizes of each signature slice
}

type storageLayerMetadata struct {
	Digest         string `json:"digest,omitempty"`
	Size           int64  `json:"size"`
	CompressedSize int64  `json:"compressed-size,omitempty"`
}

type storageImage struct {
	types.Image
	size int64
}

// newImageSource sets us up to read out an image, which needs to already exist.
func newImageSource(imageRef storageReference) (*storageImageSource, error) {
	id := imageRef.resolveID()
	if id == "" {
		logrus.Errorf("no image matching reference %q found", imageRef.StringWithinTransport())
		return nil, ErrNoSuchImage
	}
	img, err := imageRef.transport.store.GetImage(id)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading image %q", id)
	}
	image := &storageImageSource{
		imageRef:       imageRef,
		Created:        time.Now(),
		ID:             img.ID,
		BlobList:       []types.BlobInfo{},
		Layers:         make(map[ddigest.Digest][]string),
		LayerPosition:  make(map[ddigest.Digest]int),
		SignatureSizes: []int{},
	}
	if err := json.Unmarshal([]byte(img.Metadata), image); err != nil {
		return nil, errors.Wrap(err, "error decoding metadata for source image")
	}
	return image, nil
}

// newImageDestination sets us up to write a new image.
func newImageDestination(imageRef storageReference) (*storageImageDestination, error) {
	image := &storageImageDestination{
		imageRef:       imageRef,
		Tag:            imageRef.reference,
		Created:        time.Now(),
		ID:             imageRef.id,
		BlobList:       []types.BlobInfo{},
		Layers:         make(map[ddigest.Digest][]string),
		BlobData:       make(map[ddigest.Digest][]byte),
		SignatureSizes: []int{},
	}
	return image, nil
}

func (s storageImageSource) Reference() types.ImageReference {
	return s.imageRef
}

func (s storageImageDestination) Reference() types.ImageReference {
	return s.imageRef
}

func (s storageImageSource) Close() {
}

func (s storageImageDestination) Close() {
}

func (s storageImageDestination) ShouldCompressLayers() bool {
	// We ultimately have to decompress layers to populate trees on disk,
	// so callers shouldn't bother compressing them before handing them to
	// us, if they're not already compressed.
	return false
}

func (s *storageImageDestination) calculateDigest(digest ddigest.Digest) (ddigest.Digester, *ioutils.WriteCounter) {
	// Set up to read the whole blob (the initial snippet, plus the rest)
	// while digesting it with either the default, or the passed-in digest,
	// if one was specified.
	hasher := ddigest.Canonical.Digester()
	if digest.Validate() == nil {
		if a := digest.Algorithm(); a.Available() {
			hasher = a.Digester()
		}
	}
	counter := ioutils.NewWriteCounter(hasher.Hash())

	return hasher, counter
}

type storagePutParameters struct {
	multi    archive.Reader
	n        int
	blobSize int64
	header   []byte
	digest   ddigest.Digest
	blobinfo types.BlobInfo
	hasher   ddigest.Digester
	counter  *ioutils.WriteCounter
	layer    *storage.Layer
}

func (s *storageImageDestination) putLayer(id, parentLayer string, params *storagePutParameters) error {
	// Attempt to create the identified layer and import its contents.
	layer, uncompressedSize, err := s.imageRef.transport.store.PutLayer(id, parentLayer, nil, "", true, params.multi)
	params.layer = layer
	if err != nil && err != storage.ErrDuplicateID {
		logrus.Debugf("error importing layer blob %q as %q: %v", params.blobinfo.Digest, id, err)
		return err
	}
	if err == storage.ErrDuplicateID {
		// We specified an ID, and there's already a layer with
		// the same ID.  Drain the input so that we can look at
		// its length and digest.
		_, err := io.Copy(ioutil.Discard, params.multi)
		if err != nil && err != io.EOF {
			logrus.Debugf("error digesting layer blob %q: %v", params.blobinfo.Digest, id, err)
			return err
		}
	} else {
		// Applied the layer with the specified ID.  Note the
		// size info and computed digest.
		layerMeta := storageLayerMetadata{
			Digest:         params.hasher.Digest().String(),
			CompressedSize: params.counter.Count,
			Size:           uncompressedSize,
		}
		if metadata, err := json.Marshal(&layerMeta); len(metadata) != 0 && err == nil {
			s.imageRef.transport.store.SetMetadata(layer.ID, string(metadata))
		}
		// Hang on to the new layer's ID.
		id = layer.ID
	}
	params.blobSize = params.counter.Count
	// Check if the size looks right.
	if params.blobinfo.Size >= 0 && params.blobSize != params.blobinfo.Size {
		logrus.Debugf("blob %q size is %d, not %d, rejecting", params.blobinfo.Digest, params.blobSize, params.blobinfo.Size)
		if layer != nil {
			// Something's wrong; delete the newly-created layer.
			s.imageRef.transport.store.DeleteLayer(layer.ID)
		}
		return ErrBlobSizeMismatch
	}

	return nil
}

func (s *storageImageDestination) validateLayer(params *storagePutParameters) error {
	// If the content digest was specified, verify it.
	if params.digest.Validate() == nil && params.digest.String() != params.hasher.Digest().String() {
		logrus.Debugf("blob %q digests to %q, rejecting", params.blobinfo.Digest, params.hasher.Digest().String())
		if params.layer != nil {
			// Something's wrong; delete the newly-created layer.
			s.imageRef.transport.store.DeleteLayer(params.layer.ID)
		}
		return ErrBlobDigestMismatch
	}
	// If we didn't get a digest, construct one.
	if params.digest == "" {
		params.digest = ddigest.Digest(params.hasher.Digest().String())
	}

	return nil
}

func (s *storageImageDestination) processRawTarLayer(params *storagePutParameters) error {
	// It's just data.  Finish scanning it in, check that our
	// computed digest matches the passed-in digest, and store it,
	// but leave it out of the blob-to-layer-ID map so that we can
	// tell that it's not a layer.
	blob, err := ioutil.ReadAll(params.multi)
	if err != nil && err != io.EOF {
		return err
	}
	params.blobSize = int64(len(blob))
	if params.blobinfo.Size >= 0 && params.blobSize != params.blobinfo.Size {
		logrus.Debugf("blob %q size is %d, not %d, rejecting", params.blobinfo.Digest, params.blobSize, params.blobinfo.Size)
		return ErrBlobSizeMismatch
	}
	// If we were given a digest, verify that the content matches
	// it.
	if params.digest.Validate() == nil && params.digest.String() != params.hasher.Digest().String() {
		logrus.Debugf("blob %q digests to %q, rejecting", params.blobinfo.Digest, params.hasher.Digest().String())
		return ErrBlobDigestMismatch
	}
	// If we didn't get a digest, construct one.
	if params.digest == "" {
		params.digest = ddigest.Digest(params.hasher.Digest().String())
	}
	// Save the blob for when we Commit().
	s.BlobData[params.digest] = blob
	s.BlobList = append(s.BlobList, types.BlobInfo{Digest: params.digest, Size: params.blobSize})
	logrus.Debugf("blob %q imported as opaque data %q", params.blobinfo.Digest, params.digest)

	return nil
}

func (s *storageImageDestination) doPut(params *storagePutParameters) error {
	if (params.n > 1) && archive.IsArchive(params.header[:params.n]) {
		// It's a filesystem layer.  If it's not the first one in the
		// image, we assume that the most recently added layer is its
		// parent.
		parentLayer := ""
		for _, blob := range s.BlobList {
			if layerList, ok := s.Layers[blob.Digest]; ok {
				parentLayer = layerList[len(layerList)-1]
			}
		}
		// If we have an expected content digest, generate a layer ID
		// based on the parent's ID and the expected content digest.
		id := ""
		if params.digest.Validate() == nil {
			id = ddigest.Canonical.FromBytes([]byte(parentLayer + "+" + params.digest.String())).Hex()
		}

		if err := s.putLayer(id, parentLayer, params); err != nil {
			return err
		}

		if err := s.validateLayer(params); err != nil {
			return err
		}

		// Record that this layer blob is a layer, and the layer ID it
		// ended up having.  This is a list, in case the same blob is
		// being applied more than once.
		s.Layers[params.digest] = append(s.Layers[params.digest], id)
		s.BlobList = append(s.BlobList, types.BlobInfo{Digest: params.digest, Size: params.blobSize})
		if params.layer != nil {
			logrus.Debugf("blob %q imported as a filesystem layer %q", params.blobinfo.Digest, id)
		} else {
			logrus.Debugf("layer blob %q already present as layer %q", params.blobinfo.Digest, id)
		}
	} else {
		if err := s.processRawTarLayer(params); err != nil {
			return err
		}
	}

	return nil
}

// PutBlob is used to both store filesystem layers and binary data that is part
// of the image.  Filesystem layers are assumed to be imported in order, as
// that is required by some of the underlying storage drivers.
func (s *storageImageDestination) PutBlob(stream io.Reader, blobinfo types.BlobInfo) (types.BlobInfo, error) {
	blobSize := int64(-1)
	digest := blobinfo.Digest
	// Try to read an initial snippet of the blob.
	header := make([]byte, 10240)
	n, err := stream.Read(header)
	if err != nil && err != io.EOF {
		return errorBlobInfo, err
	}

	hasher, counter := s.calculateDigest(digest)

	defragmented := io.MultiReader(bytes.NewBuffer(header[:n]), stream)
	multi := io.TeeReader(defragmented, counter)

	params := &storagePutParameters{
		multi:    multi,
		header:   header,
		n:        n,
		digest:   digest,
		blobSize: blobSize,
		blobinfo: blobinfo,
		hasher:   hasher,
		counter:  counter,
	}

	err = s.doPut(params)
	if err != nil {
		return errorBlobInfo, err
	}

	return types.BlobInfo{
		Digest: params.digest,
		Size:   params.blobSize,
	}, nil
}

func (s *storageImageDestination) HasBlob(blobinfo types.BlobInfo) (bool, int64, error) {
	if blobinfo.Digest == "" {
		return false, -1, errors.Errorf(`"Can not check for a blob with unknown digest`)
	}
	for _, blob := range s.BlobList {
		if blob.Digest == blobinfo.Digest {
			return true, blob.Size, nil
		}
	}
	return false, -1, types.ErrBlobNotFound
}

func (s *storageImageDestination) ReapplyBlob(blobinfo types.BlobInfo) (types.BlobInfo, error) {
	err := blobinfo.Digest.Validate()
	if err != nil {
		return types.BlobInfo{}, err
	}
	if layerList, ok := s.Layers[blobinfo.Digest]; !ok || len(layerList) < 1 {
		b, err := s.imageRef.transport.store.GetImageBigData(s.ID, blobinfo.Digest.String())
		if err != nil {
			return types.BlobInfo{}, err
		}
		return types.BlobInfo{Digest: blobinfo.Digest, Size: int64(len(b))}, nil
	}
	layerList := s.Layers[blobinfo.Digest]
	rc, _, err := diffLayer(s.imageRef.transport.store, layerList[len(layerList)-1])
	if err != nil {
		return types.BlobInfo{}, err
	}
	return s.PutBlob(rc, blobinfo)
}

func (s *storageImageDestination) findLastLayer() string {
	lastLayer := ""
	for _, blob := range s.BlobList {
		if layerList, ok := s.Layers[blob.Digest]; ok {
			lastLayer = layerList[len(layerList)-1]
		}
	}

	return lastLayer
}

func (s *storageImageDestination) createImage(lastLayer string) (*storage.Image, error) {
	// Create the image record.
	img, err := s.imageRef.transport.store.CreateImage(s.ID, nil, lastLayer, "", nil)
	if err != nil {
		logrus.Debugf("error creating image: %q", err)
		return nil, err
	}
	logrus.Debugf("created new image ID %q", img.ID)
	s.ID = img.ID
	if s.Tag != "" {
		// We have a name to set, so move the name to this image.
		if err := s.imageRef.transport.store.SetNames(img.ID, []string{s.Tag}); err != nil {
			if _, err2 := s.imageRef.transport.store.DeleteImage(img.ID, true); err2 != nil {
				logrus.Debugf("error deleting incomplete image %q: %v", img.ID, err2)
			}
			logrus.Debugf("error setting names on image %q: %v", img.ID, err)
			return nil, err
		}
		logrus.Debugf("set name of image %q to %q", img.ID, s.Tag)
	}

	return img, nil
}

func (s *storageImageDestination) Commit() error {
	lastLayer := s.findLastLayer()
	img, err := s.createImage(lastLayer)
	if err != nil {
		return err
	}

	// Save the data blobs to disk, and drop their contents from memory.
	keys := []ddigest.Digest{}
	for k, v := range s.BlobData {
		if err := s.imageRef.transport.store.SetImageBigData(img.ID, k.String(), v); err != nil {
			if _, err2 := s.imageRef.transport.store.DeleteImage(img.ID, true); err2 != nil {
				logrus.Debugf("error deleting incomplete image %q: %v", img.ID, err2)
			}
			logrus.Debugf("error saving big data %q for image %q: %v", k, img.ID, err)
			return err
		}
		keys = append(keys, k)
	}
	for _, key := range keys {
		delete(s.BlobData, key)
	}
	// Save the manifest, if we have one.
	if err := s.imageRef.transport.store.SetImageBigData(s.ID, "manifest", s.Manifest); err != nil {
		if _, err2 := s.imageRef.transport.store.DeleteImage(img.ID, true); err2 != nil {
			logrus.Debugf("error deleting incomplete image %q: %v", img.ID, err2)
		}
		logrus.Debugf("error saving manifest for image %q: %v", img.ID, err)
		return err
	}
	// Save the signatures, if we have any.
	if err := s.imageRef.transport.store.SetImageBigData(s.ID, "signatures", s.Signatures); err != nil {
		if _, err2 := s.imageRef.transport.store.DeleteImage(img.ID, true); err2 != nil {
			logrus.Debugf("error deleting incomplete image %q: %v", img.ID, err2)
		}
		logrus.Debugf("error saving signatures for image %q: %v", img.ID, err)
		return err
	}
	// Save our metadata.
	metadata, err := json.Marshal(s)
	if err != nil {
		if _, err2 := s.imageRef.transport.store.DeleteImage(img.ID, true); err2 != nil {
			logrus.Debugf("error deleting incomplete image %q: %v", img.ID, err2)
		}
		logrus.Debugf("error encoding metadata for image %q: %v", img.ID, err)
		return err
	}
	if len(metadata) != 0 {
		if err = s.imageRef.transport.store.SetMetadata(s.ID, string(metadata)); err != nil {
			if _, err2 := s.imageRef.transport.store.DeleteImage(img.ID, true); err2 != nil {
				logrus.Debugf("error deleting incomplete image %q: %v", img.ID, err2)
			}
			logrus.Debugf("error saving metadata for image %q: %v", img.ID, err)
			return err
		}
		logrus.Debugf("saved image metadata %q", string(metadata))
	}
	return nil
}

func (s *storageImageDestination) SupportedManifestMIMETypes() []string {
	return nil
}

func (s *storageImageDestination) PutManifest(manifest []byte) error {
	s.Manifest = make([]byte, len(manifest))
	copy(s.Manifest, manifest)
	return nil
}

// SupportsSignatures returns an error if we can't expect GetSignatures() to
// return data that was previously supplied to PutSignatures().
func (s *storageImageDestination) SupportsSignatures() error {
	return nil
}

// AcceptsForeignLayerURLs returns false iff foreign layers in manifest should be actually
// uploaded to the image destination, true otherwise.
func (s *storageImageDestination) AcceptsForeignLayerURLs() bool {
	return false
}

func (s *storageImageDestination) PutSignatures(signatures [][]byte) error {
	sizes := []int{}
	sigblob := []byte{}
	for _, sig := range signatures {
		sizes = append(sizes, len(sig))
		newblob := make([]byte, len(sigblob)+len(sig))
		copy(newblob, sigblob)
		copy(newblob[len(sigblob):], sig)
		sigblob = newblob
	}
	s.Signatures = sigblob
	s.SignatureSizes = sizes
	return nil
}

func (s *storageImageSource) GetBlob(info types.BlobInfo) (rc io.ReadCloser, n int64, err error) {
	rc, n, _, err = s.getBlobAndLayerID(info)
	return rc, n, err
}

func (s *storageImageSource) getBlobAndLayerID(info types.BlobInfo) (rc io.ReadCloser, n int64, layerID string, err error) {
	err = info.Digest.Validate()
	if err != nil {
		return nil, -1, "", err
	}
	if layerList, ok := s.Layers[info.Digest]; !ok || len(layerList) < 1 {
		b, err := s.imageRef.transport.store.GetImageBigData(s.ID, info.Digest.String())
		if err != nil {
			return nil, -1, "", err
		}
		r := bytes.NewReader(b)
		logrus.Debugf("exporting opaque data as blob %q", info.Digest.String())
		return ioutil.NopCloser(r), int64(r.Len()), "", nil
	}
	// If the blob was "put" more than once, we have multiple layer IDs
	// which should all produce the same diff.  For the sake of tests that
	// want to make sure we created different layers each time the blob was
	// "put", though, cycle through the layers.
	layerList := s.Layers[info.Digest]
	position, ok := s.LayerPosition[info.Digest]
	if !ok {
		position = 0
	}
	s.LayerPosition[info.Digest] = (position + 1) % len(layerList)
	logrus.Debugf("exporting filesystem layer %q for blob %q", layerList[position], info.Digest)
	rc, n, err = diffLayer(s.imageRef.transport.store, layerList[position])
	return rc, n, layerList[position], err
}

func diffLayer(store storage.Store, layerID string) (rc io.ReadCloser, n int64, err error) {
	layer, err := store.GetLayer(layerID)
	if err != nil {
		return nil, -1, err
	}
	layerMeta := storageLayerMetadata{
		CompressedSize: -1,
	}
	if layer.Metadata != "" {
		if err := json.Unmarshal([]byte(layer.Metadata), &layerMeta); err != nil {
			return nil, -1, errors.Wrapf(err, "error decoding metadata for layer %q", layerID)
		}
	}
	if layerMeta.CompressedSize <= 0 {
		n = -1
	} else {
		n = layerMeta.CompressedSize
	}
	diff, err := store.Diff("", layer.ID)
	if err != nil {
		return nil, -1, err
	}
	return diff, n, nil
}

func (s *storageImageSource) GetManifest() (manifestBlob []byte, MIMEType string, err error) {
	manifestBlob, err = s.imageRef.transport.store.GetImageBigData(s.ID, "manifest")
	return manifestBlob, manifest.GuessMIMEType(manifestBlob), err
}

func (s *storageImageSource) GetTargetManifest(digest ddigest.Digest) (manifestBlob []byte, MIMEType string, err error) {
	return nil, "", ErrNoManifestLists
}

func (s *storageImageSource) GetSignatures() (signatures [][]byte, err error) {
	var offset int
	signature, err := s.imageRef.transport.store.GetImageBigData(s.ID, "signatures")
	if err != nil {
		return nil, err
	}
	sigslice := [][]byte{}
	for _, length := range s.SignatureSizes {
		sigslice = append(sigslice, signature[offset:offset+length])
		offset += length
	}
	if offset != len(signature) {
		return nil, errors.Errorf("signatures data contained %d extra bytes", len(signatures)-offset)
	}
	return sigslice, nil
}

func (s *storageImageSource) getSize() (int64, error) {
	var sum int64
	names, err := s.imageRef.transport.store.ListImageBigData(s.imageRef.id)
	if err != nil {
		return -1, errors.Wrapf(err, "error reading image %q", s.imageRef.id)
	}
	for _, name := range names {
		bigSize, err := s.imageRef.transport.store.GetImageBigDataSize(s.imageRef.id, name)
		if err != nil {
			return -1, errors.Wrapf(err, "error reading data blob size %q for %q", name, s.imageRef.id)
		}
		sum += bigSize
	}
	for _, sigSize := range s.SignatureSizes {
		sum += int64(sigSize)
	}
	for _, layerList := range s.Layers {
		for _, layerID := range layerList {
			layer, err := s.imageRef.transport.store.GetLayer(layerID)
			if err != nil {
				return -1, err
			}
			layerMeta := storageLayerMetadata{
				Size: -1,
			}
			if layer.Metadata != "" {
				if err := json.Unmarshal([]byte(layer.Metadata), &layerMeta); err != nil {
					return -1, errors.Wrapf(err, "error decoding metadata for layer %q", layerID)
				}
			}
			if layerMeta.Size < 0 {
				return -1, errors.Errorf("size for layer %q is unknown, failing getSize()", layerID)
			}
			sum += layerMeta.Size
		}
	}
	return sum, nil
}

func (s *storageImage) Size() (int64, error) {
	return s.size, nil
}

// newImage creates an image that also knows its size
func newImage(s storageReference) (types.Image, error) {
	src, err := newImageSource(s)
	if err != nil {
		return nil, err
	}
	img, err := image.FromSource(src)
	if err != nil {
		return nil, err
	}
	size, err := src.getSize()
	if err != nil {
		return nil, err
	}
	return &storageImage{Image: img, size: size}, nil
}

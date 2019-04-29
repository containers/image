// +build !containers_image_storage_stub

package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/containers/image/docker/reference"
	"github.com/containers/image/image"
	"github.com/containers/image/internal/tmpdir"
	"github.com/containers/image/manifest"
	"github.com/containers/image/pkg/blobinfocache/none"
	"github.com/containers/image/pkg/progress"
	"github.com/containers/image/types"
	"github.com/containers/storage"
	"github.com/containers/storage/pkg/archive"
	"github.com/containers/storage/pkg/ioutils"
	digest "github.com/opencontainers/go-digest"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var (
	// ErrBlobDigestMismatch is returned when PutBlob() is given a blob
	// with a digest-based name that doesn't match its contents.
	ErrBlobDigestMismatch = errors.New("blob digest mismatch")
	// ErrBlobSizeMismatch is returned when PutBlob() is given a blob
	// with an expected size that doesn't match the reader.
	ErrBlobSizeMismatch = errors.New("blob size mismatch")
	// ErrNoManifestLists is returned when GetManifest() is called.
	// with a non-nil instanceDigest.
	ErrNoManifestLists = errors.New("manifest lists are not supported by this transport")
	// ErrNoSuchImage is returned when we attempt to access an image which
	// doesn't exist in the storage area.
	ErrNoSuchImage = storage.ErrNotAnImage
)

type storageImageSource struct {
	imageRef       storageReference
	image          *storage.Image
	layerPosition  map[digest.Digest]int // Where we are in reading a blob's layers
	cachedManifest []byte                // A cached copy of the manifest, if already known, or nil
	getBlobMutex   sync.Mutex            // Mutex to sync state for parallel GetBlob executions
	SignatureSizes []int                 `json:"signature-sizes,omitempty"` // List of sizes of each signature slice
}

type storageImageDestination struct {
	imageRef           storageReference
	directory          string                          // Temporary directory where we store blobs until Commit() time
	nextTempFileID     int32                           // A counter that we use for computing filenames to assign to blobs
	manifest           []byte                          // Manifest contents, temporary
	signatures         []byte                          // Signature contents, temporary
	putBlobMutex       sync.Mutex                      // Mutex to sync state for parallel PutBlob executions
	blobDiffIDs        map[digest.Digest]digest.Digest // Mapping from layer blobsums to their corresponding DiffIDs
	blobLayerIDs       map[digest.Digest]string        // Mapping from layer blobsums to their corresponding storage layer ID
	fileSizes          map[digest.Digest]int64         // Mapping from layer blobsums to their sizes
	filenames          map[digest.Digest]string        // Mapping from layer blobsums to names of files we used to hold them
	indexToStorageID   map[int]string                  // Mapping from layer index to the layer IDs in the storage. Only valid after receiving `true` from the corresponding `indexToDoneChannel`.
	indexToDoneChannel map[int]chan bool               // Mapping from layer index to a channel to indicate the layer has been written to storage. True is written when the corresponding index/layer has successfully been written to the storage.
	SignatureSizes     []int                           `json:"signature-sizes,omitempty"` // List of sizes of each signature slice
}

type storageImageCloser struct {
	types.ImageCloser
	size int64
}

// manifestBigDataKey returns a key suitable for recording a manifest with the specified digest using storage.Store.ImageBigData and related functions.
// If a specific manifest digest is explicitly requested by the user, the key retruned function should be used preferably;
// for compatibility, if a manifest is not available under this key, check also storage.ImageDigestBigDataKey
func manifestBigDataKey(digest digest.Digest) string {
	return storage.ImageDigestManifestBigDataNamePrefix + "-" + digest.String()
}

// newImageSource sets up an image for reading.
func newImageSource(imageRef storageReference) (*storageImageSource, error) {
	// First, locate the image.
	img, err := imageRef.resolveImage()
	if err != nil {
		return nil, err
	}

	// Build the reader object.
	image := &storageImageSource{
		imageRef:       imageRef,
		image:          img,
		layerPosition:  make(map[digest.Digest]int),
		SignatureSizes: []int{},
	}
	if img.Metadata != "" {
		if err := json.Unmarshal([]byte(img.Metadata), image); err != nil {
			return nil, errors.Wrap(err, "error decoding metadata for source image")
		}
	}
	return image, nil
}

// Reference returns the image reference that we used to find this image.
func (s *storageImageSource) Reference() types.ImageReference {
	return s.imageRef
}

// Close cleans up any resources we tied up while reading the image.
func (s *storageImageSource) Close() error {
	return nil
}

// HasThreadSafeGetBlob indicates whether GetBlob can be executed concurrently.
func (s *storageImageSource) HasThreadSafeGetBlob() bool {
	return true
}

// GetBlob returns a stream for the specified blob, and the blob’s size (or -1 if unknown).
// The Digest field in BlobInfo is guaranteed to be provided, Size may be -1 and MediaType may be optionally provided.
// May update BlobInfoCache, preferably after it knows for certain that a blob truly exists at a specific location.
func (s *storageImageSource) GetBlob(ctx context.Context, info types.BlobInfo, cache types.BlobInfoCache) (rc io.ReadCloser, n int64, err error) {
	if info.Digest == image.GzippedEmptyLayerDigest {
		return ioutil.NopCloser(bytes.NewReader(image.GzippedEmptyLayer)), int64(len(image.GzippedEmptyLayer)), nil
	}
	rc, n, _, err = s.getBlobAndLayerID(info)
	return rc, n, err
}

// getBlobAndLayer reads the data blob or filesystem layer which matches the digest and size, if given.
func (s *storageImageSource) getBlobAndLayerID(info types.BlobInfo) (rc io.ReadCloser, n int64, layerID string, err error) {
	var layer storage.Layer
	var diffOptions *storage.DiffOptions
	// We need a valid digest value.
	err = info.Digest.Validate()
	if err != nil {
		return nil, -1, "", err
	}
	// Check if the blob corresponds to a diff that was used to initialize any layers.  Our
	// callers should try to retrieve layers using their uncompressed digests, so no need to
	// check if they're using one of the compressed digests, which we can't reproduce anyway.
	layers, err := s.imageRef.transport.store.LayersByUncompressedDigest(info.Digest)
	// If it's not a layer, then it must be a data item.
	if len(layers) == 0 {
		b, err := s.imageRef.transport.store.ImageBigData(s.image.ID, info.Digest.String())
		if err != nil {
			return nil, -1, "", err
		}
		r := bytes.NewReader(b)
		logrus.Debugf("exporting opaque data as blob %q", info.Digest.String())
		return ioutil.NopCloser(r), int64(r.Len()), "", nil
	}
	// Step through the list of matching layers.  Tests may want to verify that if we have multiple layers
	// which claim to have the same contents, that we actually do have multiple layers, otherwise we could
	// just go ahead and use the first one every time.
	s.getBlobMutex.Lock()
	i := s.layerPosition[info.Digest]
	s.layerPosition[info.Digest] = i + 1
	s.getBlobMutex.Unlock()
	if len(layers) > 0 {
		layer = layers[i%len(layers)]
	}
	// Force the storage layer to not try to match any compression that was used when the layer was first
	// handed to it.
	noCompression := archive.Uncompressed
	diffOptions = &storage.DiffOptions{
		Compression: &noCompression,
	}
	if layer.UncompressedSize < 0 {
		n = -1
	} else {
		n = layer.UncompressedSize
	}
	logrus.Debugf("exporting filesystem layer %q without compression for blob %q", layer.ID, info.Digest)
	rc, err = s.imageRef.transport.store.Diff("", layer.ID, diffOptions)
	if err != nil {
		return nil, -1, "", err
	}
	return rc, n, layer.ID, err
}

// GetManifest() reads the image's manifest.
func (s *storageImageSource) GetManifest(ctx context.Context, instanceDigest *digest.Digest) (manifestBlob []byte, MIMEType string, err error) {
	if instanceDigest != nil {
		return nil, "", ErrNoManifestLists
	}
	if len(s.cachedManifest) == 0 {
		// The manifest is stored as a big data item.
		// Prefer the manifest corresponding to the user-specified digest, if available.
		if s.imageRef.named != nil {
			if digested, ok := s.imageRef.named.(reference.Digested); ok {
				key := manifestBigDataKey(digested.Digest())
				blob, err := s.imageRef.transport.store.ImageBigData(s.image.ID, key)
				if err != nil && !os.IsNotExist(err) { // os.IsNotExist is true if the image exists but there is no data corresponding to key
					return nil, "", err
				}
				if err == nil {
					s.cachedManifest = blob
				}
			}
		}
		// If the user did not specify a digest, or this is an old image stored before manifestBigDataKey was introduced, use the default manifest.
		// Note that the manifest may not match the expected digest, and that is likely to fail eventually, e.g. in c/image/image/UnparsedImage.Manifest().
		if len(s.cachedManifest) == 0 {
			cachedBlob, err := s.imageRef.transport.store.ImageBigData(s.image.ID, storage.ImageDigestBigDataKey)
			if err != nil {
				return nil, "", err
			}
			s.cachedManifest = cachedBlob
		}
	}
	return s.cachedManifest, manifest.GuessMIMEType(s.cachedManifest), err
}

// LayerInfosForCopy() returns the list of layer blobs that make up the root filesystem of
// the image, after they've been decompressed.
func (s *storageImageSource) LayerInfosForCopy(ctx context.Context) ([]types.BlobInfo, error) {
	manifestBlob, manifestType, err := s.GetManifest(ctx, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading image manifest for %q", s.image.ID)
	}
	man, err := manifest.FromBlob(manifestBlob, manifestType)
	if err != nil {
		return nil, errors.Wrapf(err, "error parsing image manifest for %q", s.image.ID)
	}

	uncompressedLayerType := ""
	switch manifestType {
	case imgspecv1.MediaTypeImageManifest:
		uncompressedLayerType = imgspecv1.MediaTypeImageLayer
	case manifest.DockerV2Schema1MediaType, manifest.DockerV2Schema1SignedMediaType, manifest.DockerV2Schema2MediaType:
		// This is actually a compressed type, but there's no uncompressed type defined
		uncompressedLayerType = manifest.DockerV2Schema2LayerMediaType
	}

	physicalBlobInfos := []types.BlobInfo{}
	layerID := s.image.TopLayer
	for layerID != "" {
		layer, err := s.imageRef.transport.store.Layer(layerID)
		if err != nil {
			return nil, errors.Wrapf(err, "error reading layer %q in image %q", layerID, s.image.ID)
		}
		if layer.UncompressedDigest == "" {
			return nil, errors.Errorf("uncompressed digest for layer %q is unknown", layerID)
		}
		if layer.UncompressedSize < 0 {
			return nil, errors.Errorf("uncompressed size for layer %q is unknown", layerID)
		}
		blobInfo := types.BlobInfo{
			Digest:    layer.UncompressedDigest,
			Size:      layer.UncompressedSize,
			MediaType: uncompressedLayerType,
		}
		physicalBlobInfos = append([]types.BlobInfo{blobInfo}, physicalBlobInfos...)
		layerID = layer.Parent
	}

	res, err := buildLayerInfosForCopy(man.LayerInfos(), physicalBlobInfos)
	if err != nil {
		return nil, errors.Wrapf(err, "error creating LayerInfosForCopy of image %q", s.image.ID)
	}
	return res, nil
}

// buildLayerInfosForCopy builds a LayerInfosForCopy return value based on manifestInfos from the original manifest,
// but using layer data which we can actually produce — physicalInfos for non-empty layers,
// and image.GzippedEmptyLayer for empty ones.
// (This is split basically only to allow easily unit-testing the part that has no dependencies on the external environment.)
func buildLayerInfosForCopy(manifestInfos []manifest.LayerInfo, physicalInfos []types.BlobInfo) ([]types.BlobInfo, error) {
	nextPhysical := 0
	res := make([]types.BlobInfo, len(manifestInfos))
	for i, mi := range manifestInfos {
		if mi.EmptyLayer {
			res[i] = types.BlobInfo{
				Digest:    image.GzippedEmptyLayerDigest,
				Size:      int64(len(image.GzippedEmptyLayer)),
				MediaType: mi.MediaType,
			}
		} else {
			if nextPhysical >= len(physicalInfos) {
				return nil, fmt.Errorf("expected more than %d physical layers to exist", len(physicalInfos))
			}
			res[i] = physicalInfos[nextPhysical]
			nextPhysical++
		}
	}
	if nextPhysical != len(physicalInfos) {
		return nil, fmt.Errorf("used only %d out of %d physical layers", nextPhysical, len(physicalInfos))
	}
	return res, nil
}

// GetSignatures() parses the image's signatures blob into a slice of byte slices.
func (s *storageImageSource) GetSignatures(ctx context.Context, instanceDigest *digest.Digest) (signatures [][]byte, err error) {
	if instanceDigest != nil {
		return nil, ErrNoManifestLists
	}
	var offset int
	sigslice := [][]byte{}
	signature := []byte{}
	if len(s.SignatureSizes) > 0 {
		signatureBlob, err := s.imageRef.transport.store.ImageBigData(s.image.ID, "signatures")
		if err != nil {
			return nil, errors.Wrapf(err, "error looking up signatures data for image %q", s.image.ID)
		}
		signature = signatureBlob
	}
	for _, length := range s.SignatureSizes {
		sigslice = append(sigslice, signature[offset:offset+length])
		offset += length
	}
	if offset != len(signature) {
		return nil, errors.Errorf("signatures data contained %d extra bytes", len(signatures)-offset)
	}
	return sigslice, nil
}

// newImageDestination sets us up to write a new image, caching blobs in a temporary directory until
// it's time to Commit() the image
func newImageDestination(imageRef storageReference) (*storageImageDestination, error) {
	directory, err := ioutil.TempDir(tmpdir.TemporaryDirectoryForBigFiles(), "storage")
	if err != nil {
		return nil, errors.Wrapf(err, "error creating a temporary directory")
	}
	image := &storageImageDestination{
		imageRef:           imageRef,
		directory:          directory,
		blobDiffIDs:        make(map[digest.Digest]digest.Digest),
		blobLayerIDs:       make(map[digest.Digest]string),
		fileSizes:          make(map[digest.Digest]int64),
		filenames:          make(map[digest.Digest]string),
		indexToStorageID:   make(map[int]string),
		indexToDoneChannel: make(map[int]chan bool),
		SignatureSizes:     []int{},
	}
	return image, nil
}

func (s *storageImageDestination) getChannelForLayer(layerIndexInImage int) chan bool {
	s.putBlobMutex.Lock()
	defer s.putBlobMutex.Unlock()
	channel, ok := s.indexToDoneChannel[layerIndexInImage]
	if !ok {
		// A buffered channel to allow non-blocking sends
		channel = make(chan bool, 1)
		s.indexToDoneChannel[layerIndexInImage] = channel
	}
	return channel
}

// Reference returns the reference used to set up this destination.  Note that this should directly correspond to user's intent,
// e.g. it should use the public hostname instead of the result of resolving CNAMEs or following redirects.
func (s *storageImageDestination) Reference() types.ImageReference {
	return s.imageRef
}

// Close cleans up the temporary directory.
func (s *storageImageDestination) Close() error {
	return os.RemoveAll(s.directory)
}

func (s *storageImageDestination) DesiredLayerCompression() types.LayerCompression {
	// We ultimately have to decompress layers to populate trees on disk,
	// so callers shouldn't bother compressing them before handing them to
	// us, if they're not already compressed.
	return types.PreserveOriginal
}

func (s *storageImageDestination) computeNextBlobCacheFile() string {
	return filepath.Join(s.directory, fmt.Sprintf("%d", atomic.AddInt32(&s.nextTempFileID, 1)))
}

// HasThreadSafePutBlob indicates whether PutBlob can be executed concurrently.
func (s *storageImageDestination) HasThreadSafePutBlob() bool {
	return true
}

// tryReusingBlobFromOtherProcess is implementing a mechanism to detect if
// another process is already copying the blobinfo by making use of the
// blob-digest locks from containers/storage. The caller is expected to send a
// message through the done channel once the lock has been acquired. If we
// detect that another process is copying the blob, we wait until we own the
// lockfile and call tryReusingBlob() to check if we can reuse the committed
// layer.
func (s *storageImageDestination) tryReusingBlobFromOtherProcess(ctx context.Context, stream io.Reader, blobinfo types.BlobInfo, layerIndexInImage int, cache types.BlobInfoCache, done chan bool, bar *progress.Bar) (bool, types.BlobInfo, error) {
	copiedByAnotherProcess := false
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		logrus.Debugf("blob %q is being copied by another process", blobinfo.Digest)
		copiedByAnotherProcess = true
		if bar != nil {
			bar = bar.ReplaceBar(
				progress.DigestToCopyAction(blobinfo.Digest, "blob"),
				blobinfo.Size,
				progress.BarOptions{
					StaticMessage:      "paused: being copied by another process",
					RemoveOnCompletion: true,
				})
		}
		// Wait until we acquired the lock or encountered an error.
		<-done
	}

	// Now, we own the lock.
	//
	// In case another process copied the blob, we should attempt reusing the
	// blob.  If it's not available, either the copy-detection heuristic failed
	// (i.e., waiting 200 ms was not enough) or the other process failed to copy
	// the blob.
	if copiedByAnotherProcess {
		reusable, blob, err := s.tryReusingBlob(ctx, blobinfo, layerIndexInImage, cache, false)
		// If we can reuse the blob or hit an error trying to do so, we need to
		// signal the result through the channel as another Goroutine is potentially
		// waiting for it.  If we can't resuse the blob and encountered no error, we
		// need to copy it and will send the signal in PutBlob().
		if reusable {
			logrus.Debugf("another process successfully copied blob %q", blobinfo.Digest)
			if bar != nil {
				bar = bar.ReplaceBar(
					progress.DigestToCopyAction(blobinfo.Digest, "blob"),
					blobinfo.Size,
					progress.BarOptions{
						StaticMessage: "done: copied by another process",
					})
			}
		} else {
			logrus.Debugf("another process must have failed copying blob %q", blobinfo.Digest)
		}
		return reusable, blob, err
	}

	return false, types.BlobInfo{}, nil
}

// PutBlob writes contents of stream and returns data representing the result.
// inputInfo.Digest can be optionally provided if known; it is not mandatory for the implementation to verify it.
// inputInfo.Size is the expected length of stream, if known.
// inputInfo.MediaType describes the blob format, if known.
// May update cache.
// layerIndexInImage must be properly set to the layer index of the corresponding blob in the image. This value is required to allow parallel executions of
// layerIndexInImage is set to the layer index of the corresponding blob in the image. This value is required to allow parallel executions of
// PutBlob() and TryReusingBlob() where the layers must be written to the destination in sequential order. A value >= 0 indicates that the blob is a layer.
// The bar can optionally be specified to allow replacing/updating it. Note that only the containers-storage transport updates the bar; other transports ignore it.
// Same applies to bar, which is used in the containers-storage destination to update the progress bars displayed in the terminal. If it's nil, it will be ignored.
// WARNING: The contents of stream are being verified on the fly.  Until stream.Read() returns io.EOF, the contents of the data SHOULD NOT be available
// to any other readers for download using the supplied digest.
// If stream.Read() at any time, ESPECIALLY at end of input, returns an error, PutBlob MUST 1) fail, and 2) delete any data stored so far.
func (s *storageImageDestination) PutBlob(ctx context.Context, stream io.Reader, blobinfo types.BlobInfo, layerIndexInImage int, cache types.BlobInfoCache, isConfig bool, bar *progress.Bar) (blob types.BlobInfo, err error) {
	// Deferred call to an anonymous func to signal potentially waiting
	// goroutines via the index-specific channel.
	defer func() {
		// No need to wait
		if layerIndexInImage >= 0 {
			// It's a buffered channel, so we don't wait for the message to be
			// received
			channel := s.getChannelForLayer(layerIndexInImage)
			channel <- err == nil
			if err != nil {
				logrus.Debugf("error while committing blob %d: %v", layerIndexInImage, err)
			}
		}
	}()

	waitAndCommit := func(blob types.BlobInfo, err error) (types.BlobInfo, error) {
		// First, wait for the previous layer to be committed
		previousID := ""
		if layerIndexInImage > 0 {
			channel := s.getChannelForLayer(layerIndexInImage - 1)
			if committed := <-channel; !committed {
				err := fmt.Errorf("committing previous layer %d failed", layerIndexInImage-1)
				return types.BlobInfo{}, err
			}
			var ok bool
			s.putBlobMutex.Lock()
			previousID, ok = s.indexToStorageID[layerIndexInImage-1]
			s.putBlobMutex.Unlock()
			if !ok {
				return types.BlobInfo{}, fmt.Errorf("error committing blob %q: could not find parent layer ID", blob.Digest.String())
			}
		}

		// Commit the blob
		if layerIndexInImage >= 0 {
			id, err := s.commitBlob(ctx, blob, previousID)
			if err == nil {
				s.putBlobMutex.Lock()
				s.blobLayerIDs[blob.Digest] = id
				s.indexToStorageID[layerIndexInImage] = id
				s.putBlobMutex.Unlock()
			} else {
				return types.BlobInfo{}, err
			}
		}
		return blob, nil
	}

	// Check if another process is already copying the blob. Please refer to the
	// doc of tryReusingBlobFromOtherProcess for further information.
	if layerIndexInImage >= 0 {
		// The reasoning for getting the locker here is to make the code paths
		// of locking and unlocking it more obvious and simple.
		locker, err := s.imageRef.transport.store.GetDigestLock(blobinfo.Digest)
		if err != nil {
			return types.BlobInfo{}, errors.Wrapf(err, "error acquiring lock for blob %q", blobinfo.Digest)
		}
		done := make(chan bool, 1)
		defer locker.Unlock()
		go func() {
			locker.Lock()
			done <- true
		}()
		reusable, blob, err := s.tryReusingBlobFromOtherProcess(ctx, stream, blobinfo, layerIndexInImage, cache, done, bar)
		if err != nil {
			return blob, err
		}
		if reusable {
			return waitAndCommit(blob, err)
		}
	}

	if bar != nil {
		kind := "blob"
		message := ""
		if isConfig {
			kind = "config"
			// Setting the StaticMessage for the config avoids displaying a
			// progress bar. Configs are comparatively small and quickly
			// downloaded, such that displaying a progress is more a distraction
			// than an indicator.
			message = " "
		}
		bar = bar.ReplaceBar(
			progress.DigestToCopyAction(blobinfo.Digest, kind),
			blobinfo.Size,
			progress.BarOptions{
				StaticMessage:       message,
				OnCompletionMessage: "done",
			})
		stream = bar.ProxyReader(stream)
	}

	blob, err = s.putBlob(ctx, stream, blobinfo, layerIndexInImage, cache, isConfig)
	if err != nil {
		return types.BlobInfo{}, err
	}

	return waitAndCommit(blob, err)
}

func (s *storageImageDestination) putBlob(ctx context.Context, stream io.Reader, blobinfo types.BlobInfo, layerIndexInImage int, cache types.BlobInfoCache, isConfig bool) (types.BlobInfo, error) {
	// Stores a layer or data blob in our temporary directory, checking that any information
	// in the blobinfo matches the incoming data.
	errorBlobInfo := types.BlobInfo{
		Digest: "",
		Size:   -1,
	}
	// Set up to digest the blob and count its size while saving it to a file.
	hasher := digest.Canonical.Digester()
	if blobinfo.Digest.Validate() == nil {
		if a := blobinfo.Digest.Algorithm(); a.Available() {
			hasher = a.Digester()
		}
	}
	diffID := digest.Canonical.Digester()
	filename := s.computeNextBlobCacheFile()
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_TRUNC|os.O_WRONLY|os.O_EXCL, 0600)
	if err != nil {
		return errorBlobInfo, errors.Wrapf(err, "error creating temporary file %q", filename)
	}
	defer file.Close()
	counter := ioutils.NewWriteCounter(hasher.Hash())
	reader := io.TeeReader(io.TeeReader(stream, counter), file)
	decompressed, err := archive.DecompressStream(reader)
	if err != nil {
		return errorBlobInfo, errors.Wrap(err, "error setting up to decompress blob")
	}
	// Copy the data to the file.
	// TODO: This can take quite some time, and should ideally be cancellable using ctx.Done().
	_, err = io.Copy(diffID.Hash(), decompressed)
	decompressed.Close()
	if err != nil {
		return errorBlobInfo, errors.Wrapf(err, "error storing blob to file %q", filename)
	}
	// Ensure that any information that we were given about the blob is correct.
	if blobinfo.Digest.Validate() == nil && blobinfo.Digest != hasher.Digest() {
		return errorBlobInfo, ErrBlobDigestMismatch
	}
	if blobinfo.Size >= 0 && blobinfo.Size != counter.Count {
		return errorBlobInfo, ErrBlobSizeMismatch
	}
	// Record information about the blob.
	s.putBlobMutex.Lock()
	s.blobDiffIDs[hasher.Digest()] = diffID.Digest()
	s.fileSizes[hasher.Digest()] = counter.Count
	s.filenames[hasher.Digest()] = filename
	s.putBlobMutex.Unlock()
	blobDigest := blobinfo.Digest
	if blobDigest.Validate() != nil {
		blobDigest = hasher.Digest()
	}
	blobSize := blobinfo.Size
	if blobSize < 0 {
		blobSize = counter.Count
	}
	// This is safe because we have just computed both values ourselves.
	cache.RecordDigestUncompressedPair(blobDigest, diffID.Digest())
	blob := types.BlobInfo{
		Digest:    blobDigest,
		Size:      blobSize,
		MediaType: blobinfo.MediaType,
	}

	return blob, nil
}

// tryReusingBlob is a helper method for TryReusingBlob to wrap it
func (s *storageImageDestination) tryReusingBlob(ctx context.Context, blobinfo types.BlobInfo, layerIndexInImage int, cache types.BlobInfoCache, canSubstitute bool) (bool, types.BlobInfo, error) {
	// lock the entire method as it executes fairly quickly
	s.putBlobMutex.Lock()
	defer s.putBlobMutex.Unlock()
	if blobinfo.Digest == "" {
		return false, types.BlobInfo{}, errors.Errorf(`Can not check for a blob with unknown digest`)
	}
	if err := blobinfo.Digest.Validate(); err != nil {
		return false, types.BlobInfo{}, errors.Wrapf(err, `Can not check for a blob with invalid digest`)
	}

	// Check if we've already cached it in a file.
	if size, ok := s.fileSizes[blobinfo.Digest]; ok {
		return true, types.BlobInfo{
			Digest:    blobinfo.Digest,
			Size:      size,
			MediaType: blobinfo.MediaType,
		}, nil
	}

	// Check if we have a wasn't-compressed layer in storage that's based on that blob.
	layers, err := s.imageRef.transport.store.LayersByUncompressedDigest(blobinfo.Digest)
	if err != nil && errors.Cause(err) != storage.ErrLayerUnknown {
		return false, types.BlobInfo{}, errors.Wrapf(err, `Error looking for layers with digest %q`, blobinfo.Digest)
	}
	if len(layers) > 0 {
		// Save this for completeness.
		s.blobDiffIDs[blobinfo.Digest] = layers[0].UncompressedDigest
		s.blobLayerIDs[blobinfo.Digest] = layers[0].ID
		if layerIndexInImage >= 0 {
			s.indexToStorageID[layerIndexInImage] = layers[0].ID
		}
		return true, types.BlobInfo{
			Digest:    blobinfo.Digest,
			Size:      layers[0].UncompressedSize,
			MediaType: blobinfo.MediaType,
		}, nil
	}

	// Check if we have a was-compressed layer in storage that's based on that blob.
	layers, err = s.imageRef.transport.store.LayersByCompressedDigest(blobinfo.Digest)
	if err != nil && errors.Cause(err) != storage.ErrLayerUnknown {
		return false, types.BlobInfo{}, errors.Wrapf(err, `Error looking for compressed layers with digest %q`, blobinfo.Digest)
	}
	if len(layers) > 0 {
		// Record the uncompressed value so that we can use it to calculate layer IDs.
		s.blobDiffIDs[blobinfo.Digest] = layers[0].UncompressedDigest
		s.blobLayerIDs[blobinfo.Digest] = layers[0].ID
		if layerIndexInImage >= 0 {
			s.indexToStorageID[layerIndexInImage] = layers[0].ID
		}
		return true, types.BlobInfo{
			Digest:    blobinfo.Digest,
			Size:      layers[0].CompressedSize,
			MediaType: blobinfo.MediaType,
		}, nil
	}

	// Does the blob correspond to a known DiffID which we already have available?
	// Because we must return the size, which is unknown for unavailable compressed blobs, the returned BlobInfo refers to the
	// uncompressed layer, and that can happen only if canSubstitute.
	if canSubstitute {
		if uncompressedDigest := cache.UncompressedDigest(blobinfo.Digest); uncompressedDigest != "" && uncompressedDigest != blobinfo.Digest {
			layers, err := s.imageRef.transport.store.LayersByUncompressedDigest(uncompressedDigest)
			if err != nil && errors.Cause(err) != storage.ErrLayerUnknown {
				return false, types.BlobInfo{}, errors.Wrapf(err, `Error looking for layers with digest %q`, uncompressedDigest)
			}
			if len(layers) > 0 {
				s.blobDiffIDs[uncompressedDigest] = layers[0].UncompressedDigest
				s.blobLayerIDs[blobinfo.Digest] = layers[0].ID
				if layerIndexInImage >= 0 {
					s.indexToStorageID[layerIndexInImage] = layers[0].ID
				}
				return true, types.BlobInfo{
					Digest:    uncompressedDigest,
					Size:      layers[0].UncompressedSize,
					MediaType: blobinfo.MediaType,
				}, nil
			}
		}
	}

	// Nope, we don't have it.
	return false, types.BlobInfo{}, nil
}

// TryReusingBlob checks whether the transport already contains, or can efficiently reuse, a blob, and if so, applies it to the current destination
// (e.g. if the blob is a filesystem layer, this signifies that the changes it describes need to be applied again when composing a filesystem tree).
// info.Digest must not be empty.
// layerIndexInImage must be properly set to the layer index of the corresponding blob in the image. This value is required to allow parallel executions of
// PutBlob() and TryReusingBlob() where the layers must be written to the backend storage in sequential order. A value >= indicates that the blob a layer.
// Note that only the containers-storage destination is sensitive to the layerIndexInImage parameter. Other transport destinations ignore it.
// Same applies to bar, which is used in the containers-storage destination to update the progress bars displayed in the terminal. If it's nil, it will be ignored.
// If canSubstitute, TryReusingBlob can use an equivalent equivalent of the desired blob; in that case the returned info may not match the input.
// If the blob has been successfully reused, returns (true, info, nil); info must contain at least a digest and size.
// If the transport can not reuse the requested blob, TryReusingBlob returns (false, {}, nil); it returns a non-nil error only on an unexpected failure.
// May use and/or update cache.
func (s *storageImageDestination) TryReusingBlob(ctx context.Context, blobinfo types.BlobInfo, layerIndexInImage int, cache types.BlobInfoCache, canSubstitute bool, bar *progress.Bar) (bool, types.BlobInfo, error) {
	reusable, blob, err := s.tryReusingBlob(ctx, blobinfo, layerIndexInImage, cache, canSubstitute)
	// If we can reuse the blob or hit an error trying to do so, we need to
	// signal the result through the channel as another Goroutine is potentially
	// waiting for it.  If we can't resuse the blob and encountered no error, we
	// need to copy it and will send the signal in PutBlob().
	if (layerIndexInImage >= 0) && (err != nil || reusable) {
		channel := s.getChannelForLayer(layerIndexInImage)
		channel <- (err == nil)
	}
	if bar != nil && reusable {
		bar.ReplaceBar(
			progress.DigestToCopyAction(blobinfo.Digest, "blob"),
			0,
			progress.BarOptions{
				StaticMessage: "skipped: already exists",
			})
	}
	return reusable, blob, err
}

// computeID computes a recommended image ID based on information we have so far.  If
// the manifest is not of a type that we recognize, we return an empty value, indicating
// that since we don't have a recommendation, a random ID should be used if one needs
// to be allocated.
func (s *storageImageDestination) computeID(m manifest.Manifest) string {
	// Build the diffID list.  We need the decompressed sums that we've been calculating to
	// fill in the DiffIDs.  It's expected (but not enforced by us) that the number of
	// diffIDs corresponds to the number of non-EmptyLayer entries in the history.
	var diffIDs []digest.Digest
	switch m := m.(type) {
	case *manifest.Schema1:
		// Build a list of the diffIDs we've generated for the non-throwaway FS layers,
		// in reverse of the order in which they were originally listed.
		for i, compat := range m.ExtractedV1Compatibility {
			if compat.ThrowAway {
				continue
			}
			blobSum := m.FSLayers[i].BlobSum
			diffID, ok := s.blobDiffIDs[blobSum]
			if !ok {
				logrus.Infof("error looking up diffID for layer %q", blobSum.String())
				return ""
			}
			diffIDs = append([]digest.Digest{diffID}, diffIDs...)
		}
	case *manifest.Schema2, *manifest.OCI1:
		// We know the ID calculation for these formats doesn't actually use the diffIDs,
		// so we don't need to populate the diffID list.
	default:
		return ""
	}
	id, err := m.ImageID(diffIDs)
	if err != nil {
		return ""
	}
	return id
}

// getConfigBlob exists only to let us retrieve the configuration blob so that the manifest package can dig
// information out of it for Inspect().
func (s *storageImageDestination) getConfigBlob(info types.BlobInfo) ([]byte, error) {
	if info.Digest == "" {
		return nil, errors.Errorf(`no digest supplied when reading blob`)
	}
	if err := info.Digest.Validate(); err != nil {
		return nil, errors.Wrapf(err, `invalid digest supplied when reading blob`)
	}
	// Assume it's a file, since we're only calling this from a place that expects to read files.
	if filename, ok := s.filenames[info.Digest]; ok {
		contents, err2 := ioutil.ReadFile(filename)
		if err2 != nil {
			return nil, errors.Wrapf(err2, `error reading blob from file %q`, filename)
		}
		return contents, nil
	}
	// If it's not a file, it's a bug, because we're not expecting to be asked for a layer.
	return nil, errors.New("blob not found")
}

// commitBlobs commits the specified blob to the storage. If not already done,
// it will block until the parent layer is committed to the storage. Note that
// the only caller of commitBlob() is PutBlob(), which is recording the results
// and send the error through the corresponding channel in s.previousLayerResult.
func (s *storageImageDestination) commitBlob(ctx context.Context, blob types.BlobInfo, previousID string) (string, error) {
	logrus.Debugf("committing blob %q", blob.Digest)
	// Check if there's already a layer with the ID that we'd give to the result of applying
	// this layer blob to its parent, if it has one, or the blob's hex value otherwise.
	s.putBlobMutex.Lock()
	diffID, haveDiffID := s.blobDiffIDs[blob.Digest]
	s.putBlobMutex.Unlock()
	if !haveDiffID {
		return "", errors.Errorf("we have blob %q, but don't know its uncompressed digest", blob.Digest.String())
	}

	id := diffID.Hex()
	if previousID != "" {
		id = digest.Canonical.FromBytes([]byte(previousID + "+" + diffID.Hex())).Hex()
	}
	if layer, err2 := s.imageRef.transport.store.Layer(id); layer != nil && err2 == nil {
		logrus.Debugf("layer for blob %q already found in storage", blob.Digest)
		return layer.ID, nil
	}
	// Check if we previously cached a file with that blob's contents.  If we didn't,
	// then we need to read the desired contents from a layer.
	s.putBlobMutex.Lock()
	filename, ok := s.filenames[blob.Digest]
	s.putBlobMutex.Unlock()
	if !ok {
		// Try to find the layer with contents matching that blobsum.
		layer := ""
		layers, err2 := s.imageRef.transport.store.LayersByUncompressedDigest(blob.Digest)
		if err2 == nil && len(layers) > 0 {
			layer = layers[0].ID
		} else {
			layers, err2 = s.imageRef.transport.store.LayersByCompressedDigest(blob.Digest)
			if err2 == nil && len(layers) > 0 {
				layer = layers[0].ID
			}
		}
		if layer == "" {
			return "", errors.Wrapf(err2, "error locating layer for blob %q", blob.Digest)
		}
		// Read the layer's contents.
		noCompression := archive.Uncompressed
		diffOptions := &storage.DiffOptions{
			Compression: &noCompression,
		}
		diff, err2 := s.imageRef.transport.store.Diff("", layer, diffOptions)
		if err2 != nil {
			return "", errors.Wrapf(err2, "error reading layer %q for blob %q", layer, blob.Digest)
		}
		// Copy the layer diff to a file.  Diff() takes a lock that it holds
		// until the ReadCloser that it returns is closed, and PutLayer() wants
		// the same lock, so the diff can't just be directly streamed from one
		// to the other.
		filename = s.computeNextBlobCacheFile()
		file, err := os.OpenFile(filename, os.O_CREATE|os.O_TRUNC|os.O_WRONLY|os.O_EXCL, 0600)
		if err != nil {
			diff.Close()
			return "", errors.Wrapf(err, "error creating temporary file %q", filename)
		}
		// Copy the data to the file.
		// TODO: This can take quite some time, and should ideally be cancellable using
		// ctx.Done().
		_, err = io.Copy(file, diff)
		diff.Close()
		file.Close()
		if err != nil {
			return "", errors.Wrapf(err, "error storing blob to file %q", filename)
		}
		// Make sure that we can find this file later, should we need the layer's
		// contents again.
		s.putBlobMutex.Lock()
		s.filenames[blob.Digest] = filename
		s.putBlobMutex.Unlock()
	}
	// Read the cached blob and use it as a diff.
	file, err := os.Open(filename)
	if err != nil {
		return "", errors.Wrapf(err, "error opening file %q", filename)
	}
	defer file.Close()
	// Build the new layer using the diff, regardless of where it came from.
	// TODO: This can take quite some time, and should ideally be cancellable using ctx.Done().
	layer, _, err := s.imageRef.transport.store.PutLayer(id, previousID, nil, "", false, nil, file)
	if err != nil && errors.Cause(err) != storage.ErrDuplicateID {
		return "", errors.Wrapf(err, "error adding layer with blob %q", blob.Digest)
	}
	return layer.ID, nil
}

// Commit marks the process of storing the image as successful and asks for the image to be persisted.
// WARNING: This does not have any transactional semantics:
// - Uploaded data MAY be visible to others before Commit() is called
// - Uploaded data MAY be removed or MAY remain around if Close() is called without Commit() (i.e. rollback is allowed but not guaranteed)
func (s *storageImageDestination) Commit(ctx context.Context) error {
	// Find the list of layer blobs.
	if len(s.manifest) == 0 {
		return errors.New("Internal error: storageImageDestination.Commit() called without PutManifest()")
	}
	man, err := manifest.FromBlob(s.manifest, manifest.GuessMIMEType(s.manifest))
	if err != nil {
		return errors.Wrapf(err, "error parsing manifest")
	}

	layerBlobs := man.LayerInfos()
	// Extract or find the layers.
	lastLayer := ""
	for _, blob := range layerBlobs {
		if blob.EmptyLayer {
			continue
		}
		_, haveDiffID := s.blobDiffIDs[blob.Digest]
		if !haveDiffID {
			// Check if it's elsewhere and the caller just forgot to pass it to us in a PutBlob(),
			// or to even check if we had it.
			// Use none.NoCache to avoid a repeated DiffID lookup in the BlobInfoCache; a caller
			// that relies on using a blob digest that has never been seeen by the store had better call
			// TryReusingBlob; not calling PutBlob already violates the documented API, so there’s only
			// so far we are going to accommodate that (if we should be doing that at all).
			logrus.Debugf("looking for diffID for blob %+v", blob.Digest)
			has, _, err := s.tryReusingBlob(ctx, blob.BlobInfo, -1, none.NoCache, false)
			if err != nil {
				return errors.Wrapf(err, "error checking for a layer based on blob %q", blob.Digest.String())
			}
			if !has {
				return errors.Errorf("error determining uncompressed digest for blob %q", blob.Digest.String())
			}
			_, haveDiffID = s.blobDiffIDs[blob.Digest]
			if !haveDiffID {
				return errors.Errorf("we have blob %q, but don't know its uncompressed digest", blob.Digest.String())
			}
		}
		newID, err := s.commitBlob(ctx, blob.BlobInfo, lastLayer)
		if err != nil {
			return err
		}
		lastLayer = newID
	}

	if lastLayer == "" {
		return fmt.Errorf("could not find top layer")
	}

	// If one of those blobs was a configuration blob, then we can try to dig out the date when the image
	// was originally created, in case we're just copying it.  If not, no harm done.
	options := &storage.ImageOptions{}
	if inspect, err := man.Inspect(s.getConfigBlob); err == nil && inspect.Created != nil {
		logrus.Debugf("setting image creation date to %s", inspect.Created)
		options.CreationDate = *inspect.Created
	}
	// Create the image record, pointing to the most-recently added layer.
	intendedID := s.imageRef.id
	if intendedID == "" {
		intendedID = s.computeID(man)
	}
	oldNames := []string{}
	img, err := s.imageRef.transport.store.CreateImage(intendedID, nil, lastLayer, "", options)
	if err != nil {
		if errors.Cause(err) != storage.ErrDuplicateID {
			logrus.Debugf("error creating image: %q", err)
			return errors.Wrapf(err, "error creating image %q", intendedID)
		}
		img, err = s.imageRef.transport.store.Image(intendedID)
		if err != nil {
			return errors.Wrapf(err, "error reading image %q", intendedID)
		}
		if img.TopLayer != lastLayer {
			logrus.Debugf("error creating image: image with ID %q exists, but uses different layers", intendedID)
			return errors.Wrapf(storage.ErrDuplicateID, "image with ID %q already exists, but uses a different top layer", intendedID)
		}
		logrus.Debugf("reusing image ID %q", img.ID)
		oldNames = append(oldNames, img.Names...)
	} else {
		logrus.Debugf("created new image ID %q", img.ID)
	}
	// Add the non-layer blobs as data items.  Since we only share layers, they should all be in files, so
	// we just need to screen out the ones that are actually layers to get the list of non-layers.
	dataBlobs := make(map[digest.Digest]struct{})
	for blob := range s.filenames {
		dataBlobs[blob] = struct{}{}
	}
	for _, layerBlob := range layerBlobs {
		delete(dataBlobs, layerBlob.Digest)
	}
	for blob := range dataBlobs {
		v, err := ioutil.ReadFile(s.filenames[blob])
		if err != nil {
			return errors.Wrapf(err, "error copying non-layer blob %q to image", blob)
		}
		if err := s.imageRef.transport.store.SetImageBigData(img.ID, blob.String(), v, manifest.Digest); err != nil {
			if _, err2 := s.imageRef.transport.store.DeleteImage(img.ID, true); err2 != nil {
				logrus.Debugf("error deleting incomplete image %q: %v", img.ID, err2)
			}
			logrus.Debugf("error saving big data %q for image %q: %v", blob.String(), img.ID, err)
			return errors.Wrapf(err, "error saving big data %q for image %q", blob.String(), img.ID)
		}
	}
	// Set the reference's name on the image.
	if name := s.imageRef.DockerReference(); len(oldNames) > 0 || name != nil {
		names := []string{}
		if name != nil {
			names = append(names, name.String())
		}
		if len(oldNames) > 0 {
			names = append(names, oldNames...)
		}
		if err := s.imageRef.transport.store.SetNames(img.ID, names); err != nil {
			if _, err2 := s.imageRef.transport.store.DeleteImage(img.ID, true); err2 != nil {
				logrus.Debugf("error deleting incomplete image %q: %v", img.ID, err2)
			}
			logrus.Debugf("error setting names %v on image %q: %v", names, img.ID, err)
			return errors.Wrapf(err, "error setting names %v on image %q", names, img.ID)
		}
		logrus.Debugf("set names of image %q to %v", img.ID, names)
	}
	// Save the manifest.  Allow looking it up by digest by using the key convention defined by the Store.
	// Record the manifest twice: using a digest-specific key to allow references to that specific digest instance,
	// and using storage.ImageDigestBigDataKey for future users that don’t specify any digest and for compatibility with older readers.
	manifestDigest, err := manifest.Digest(s.manifest)
	if err != nil {
		return errors.Wrapf(err, "error computing manifest digest")
	}
	if err := s.imageRef.transport.store.SetImageBigData(img.ID, manifestBigDataKey(manifestDigest), s.manifest, manifest.Digest); err != nil {
		if _, err2 := s.imageRef.transport.store.DeleteImage(img.ID, true); err2 != nil {
			logrus.Debugf("error deleting incomplete image %q: %v", img.ID, err2)
		}
		logrus.Debugf("error saving manifest for image %q: %v", img.ID, err)
		return err
	}
	if err := s.imageRef.transport.store.SetImageBigData(img.ID, storage.ImageDigestBigDataKey, s.manifest, manifest.Digest); err != nil {
		if _, err2 := s.imageRef.transport.store.DeleteImage(img.ID, true); err2 != nil {
			logrus.Debugf("error deleting incomplete image %q: %v", img.ID, err2)
		}
		logrus.Debugf("error saving manifest for image %q: %v", img.ID, err)
		return err
	}
	// Save the signatures, if we have any.
	if len(s.signatures) > 0 {
		if err := s.imageRef.transport.store.SetImageBigData(img.ID, "signatures", s.signatures, manifest.Digest); err != nil {
			if _, err2 := s.imageRef.transport.store.DeleteImage(img.ID, true); err2 != nil {
				logrus.Debugf("error deleting incomplete image %q: %v", img.ID, err2)
			}
			logrus.Debugf("error saving signatures for image %q: %v", img.ID, err)
			return err
		}
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
		if err = s.imageRef.transport.store.SetMetadata(img.ID, string(metadata)); err != nil {
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

var manifestMIMETypes = []string{
	imgspecv1.MediaTypeImageManifest,
	manifest.DockerV2Schema2MediaType,
	manifest.DockerV2Schema1SignedMediaType,
	manifest.DockerV2Schema1MediaType,
}

func (s *storageImageDestination) SupportedManifestMIMETypes() []string {
	return manifestMIMETypes
}

// PutManifest writes the manifest to the destination.
func (s *storageImageDestination) PutManifest(ctx context.Context, manifestBlob []byte) error {
	if s.imageRef.named != nil {
		if digested, ok := s.imageRef.named.(reference.Digested); ok {
			matches, err := manifest.MatchesDigest(manifestBlob, digested.Digest())
			if err != nil {
				return err
			}
			if !matches {
				return fmt.Errorf("Manifest does not match expected digest %s", digested.Digest())
			}
		}
	}

	s.manifest = make([]byte, len(manifestBlob))
	copy(s.manifest, manifestBlob)
	return nil
}

// SupportsSignatures returns an error if we can't expect GetSignatures() to return data that was
// previously supplied to PutSignatures().
func (s *storageImageDestination) SupportsSignatures(ctx context.Context) error {
	return nil
}

// AcceptsForeignLayerURLs returns false iff foreign layers in the manifest should actually be
// uploaded to the image destination, true otherwise.
func (s *storageImageDestination) AcceptsForeignLayerURLs() bool {
	return false
}

// MustMatchRuntimeOS returns true iff the destination can store only images targeted for the current runtime OS. False otherwise.
func (s *storageImageDestination) MustMatchRuntimeOS() bool {
	return true
}

// IgnoresEmbeddedDockerReference returns true iff the destination does not care about Image.EmbeddedDockerReferenceConflicts(),
// and would prefer to receive an unmodified manifest instead of one modified for the destination.
// Does not make a difference if Reference().DockerReference() is nil.
func (s *storageImageDestination) IgnoresEmbeddedDockerReference() bool {
	return true // Yes, we want the unmodified manifest
}

// PutSignatures records the image's signatures for committing as a single data blob.
func (s *storageImageDestination) PutSignatures(ctx context.Context, signatures [][]byte) error {
	sizes := []int{}
	sigblob := []byte{}
	for _, sig := range signatures {
		sizes = append(sizes, len(sig))
		newblob := make([]byte, len(sigblob)+len(sig))
		copy(newblob, sigblob)
		copy(newblob[len(sigblob):], sig)
		sigblob = newblob
	}
	s.signatures = sigblob
	s.SignatureSizes = sizes
	return nil
}

// getSize() adds up the sizes of the image's data blobs (which includes the configuration blob), the
// signatures, and the uncompressed sizes of all of the image's layers.
func (s *storageImageSource) getSize() (int64, error) {
	var sum int64
	// Size up the data blobs.
	dataNames, err := s.imageRef.transport.store.ListImageBigData(s.image.ID)
	if err != nil {
		return -1, errors.Wrapf(err, "error reading image %q", s.image.ID)
	}
	for _, dataName := range dataNames {
		bigSize, err := s.imageRef.transport.store.ImageBigDataSize(s.image.ID, dataName)
		if err != nil {
			return -1, errors.Wrapf(err, "error reading data blob size %q for %q", dataName, s.image.ID)
		}
		sum += bigSize
	}
	// Add the signature sizes.
	for _, sigSize := range s.SignatureSizes {
		sum += int64(sigSize)
	}
	// Walk the layer list.
	layerID := s.image.TopLayer
	for layerID != "" {
		layer, err := s.imageRef.transport.store.Layer(layerID)
		if err != nil {
			return -1, err
		}
		if layer.UncompressedDigest == "" || layer.UncompressedSize < 0 {
			return -1, errors.Errorf("size for layer %q is unknown, failing getSize()", layerID)
		}
		sum += layer.UncompressedSize
		if layer.Parent == "" {
			break
		}
		layerID = layer.Parent
	}
	return sum, nil
}

// Size() adds up the sizes of the image's data blobs (which includes the configuration blob), the
// signatures, and the uncompressed sizes of all of the image's layers.
func (s *storageImageSource) Size() (int64, error) {
	return s.getSize()
}

// Size() returns the previously-computed size of the image, with no error.
func (s *storageImageCloser) Size() (int64, error) {
	return s.size, nil
}

// newImage creates an image that also knows its size
func newImage(ctx context.Context, sys *types.SystemContext, s storageReference) (types.ImageCloser, error) {
	src, err := newImageSource(s)
	if err != nil {
		return nil, err
	}
	img, err := image.FromSource(ctx, sys, src)
	if err != nil {
		return nil, err
	}
	size, err := src.getSize()
	if err != nil {
		return nil, err
	}
	return &storageImageCloser{ImageCloser: img, size: size}, nil
}

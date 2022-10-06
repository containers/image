package archive

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/containers/image/v5/internal/imagesource"
	"github.com/containers/image/v5/internal/imagesource/impl"
	"github.com/containers/image/v5/internal/private"
	"github.com/containers/image/v5/internal/signature"
	"github.com/containers/image/v5/oci/layout"
	"github.com/containers/image/v5/types"
	digest "github.com/opencontainers/go-digest"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
)

type ociArchiveImageSource struct {
	impl.Compat

	ref                   ociArchiveReference
	unpackedSrc           private.ImageSource
	individualReaderOrNil *Reader
}

// openRef returns (layoutRef, individualReaderOrNil) for consuming ref.
// The caller must close individualReaderOrNil (if the latter is not nil).
func openRef(ctx context.Context, sys *types.SystemContext, ref ociArchiveReference) (types.ImageReference, *Reader, error) {
	var (
		archive, individualReaderOrNil *Reader
		layoutRef                      types.ImageReference
		err                            error
	)

	if ref.archiveReader != nil {
		archive = ref.archiveReader
		individualReaderOrNil = nil
	} else {
		archive, err = NewReader(ctx, sys, ref.resolvedFile)
		if err != nil {
			return nil, nil, err
		}
		individualReaderOrNil = archive
	}
	succeeded := false
	defer func() {
		if !succeeded && individualReaderOrNil != nil {
			individualReaderOrNil.Close()
		}
	}()

	if ref.sourceIndex != -1 {
		layoutRef, err = layout.NewIndexReference(archive.tempDirectory, ref.sourceIndex)
		if err != nil {
			return nil, nil, err
		}
	} else {
		layoutRef, err = layout.NewReference(archive.tempDirectory, ref.image)
		if err != nil {
			return nil, nil, err
		}
	}

	succeeded = true
	return layoutRef, individualReaderOrNil, nil
}

// newImageSource returns an ImageSource for reading from an existing directory.
func newImageSource(ctx context.Context, sys *types.SystemContext, ref ociArchiveReference) (private.ImageSource, error) {
	layoutRef, individualReaderOrNil, err := openRef(ctx, sys, ref)
	if err != nil {
		return nil, err
	}
	succeeded := false
	defer func() {
		if !succeeded && individualReaderOrNil != nil {
			individualReaderOrNil.Close()
		}
	}()

	src, err := layoutRef.NewImageSource(ctx, sys)
	if err != nil {
		return nil, err
	}

	succeeded = true
	s := &ociArchiveImageSource{
		ref:                   ref,
		unpackedSrc:           imagesource.FromPublic(src),
		individualReaderOrNil: individualReaderOrNil,
	}
	s.Compat = impl.AddCompat(s)
	return s, nil
}

// LoadManifestDescriptor loads the manifest
// Deprecated: use LoadManifestDescriptorWithContext instead
func LoadManifestDescriptor(imgRef types.ImageReference) (imgspecv1.Descriptor, error) {
	return LoadManifestDescriptorWithContext(nil, imgRef)
}

// LoadManifestDescriptorWithContext loads the manifest
func LoadManifestDescriptorWithContext(sys *types.SystemContext, imgRef types.ImageReference) (imgspecv1.Descriptor, error) {
	ociArchRef, ok := imgRef.(ociArchiveReference)
	if !ok {
		return imgspecv1.Descriptor{}, errors.New("error typecasting, need type ociArchiveReference")
	}

	layoutRef, individualReaderOrNil, err := openRef(context.TODO(), sys, ociArchRef)
	if err != nil {
		return imgspecv1.Descriptor{}, err
	}
	defer func() {
		if individualReaderOrNil != nil {
			if err := individualReaderOrNil.Close(); err != nil {
				logrus.Debugf("Error deleting temporary directory: %v", err)
			}
		}
	}()

	descriptor, err := layout.LoadManifestDescriptor(layoutRef)
	if err != nil {
		return imgspecv1.Descriptor{}, fmt.Errorf("loading index: %w", err)
	}
	return descriptor, nil
}

// Reference returns the reference used to set up this source.
func (s *ociArchiveImageSource) Reference() types.ImageReference {
	return s.ref
}

// Close removes resources associated with an initialized ImageSource, if any.
func (s *ociArchiveImageSource) Close() error {
	if err := s.unpackedSrc.Close(); err != nil {
		return err
	}
	if s.individualReaderOrNil == nil {
		return nil
	}
	return s.individualReaderOrNil.Close()
}

// GetManifest returns the image's manifest along with its MIME type (which may be empty when it can't be determined but the manifest is available).
// It may use a remote (= slow) service.
// If instanceDigest is not nil, it contains a digest of the specific manifest instance to retrieve (when the primary manifest is a manifest list);
// this never happens if the primary manifest is not a manifest list (e.g. if the source never returns manifest lists).
func (s *ociArchiveImageSource) GetManifest(ctx context.Context, instanceDigest *digest.Digest) ([]byte, string, error) {
	return s.unpackedSrc.GetManifest(ctx, instanceDigest)
}

// HasThreadSafeGetBlob indicates whether GetBlob can be executed concurrently.
func (s *ociArchiveImageSource) HasThreadSafeGetBlob() bool {
	return false
}

// GetBlob returns a stream for the specified blob, and the blob’s size (or -1 if unknown).
// The Digest field in BlobInfo is guaranteed to be provided, Size may be -1 and MediaType may be optionally provided.
// May update BlobInfoCache, preferably after it knows for certain that a blob truly exists at a specific location.
func (s *ociArchiveImageSource) GetBlob(ctx context.Context, info types.BlobInfo, cache types.BlobInfoCache) (io.ReadCloser, int64, error) {
	return s.unpackedSrc.GetBlob(ctx, info, cache)
}

// SupportsGetBlobAt() returns true if GetBlobAt (BlobChunkAccessor) is supported.
func (s *ociArchiveImageSource) SupportsGetBlobAt() bool {
	return s.unpackedSrc.SupportsGetBlobAt()
}

// GetBlobAt returns a sequential channel of readers that contain data for the requested
// blob chunks, and a channel that might get a single error value.
// The specified chunks must be not overlapping and sorted by their offset.
// The readers must be fully consumed, in the order they are returned, before blocking
// to read the next chunk.
func (s *ociArchiveImageSource) GetBlobAt(ctx context.Context, info types.BlobInfo, chunks []private.ImageSourceChunk) (chan io.ReadCloser, chan error, error) {
	return s.unpackedSrc.GetBlobAt(ctx, info, chunks)
}

// GetSignaturesWithFormat returns the image's signatures.  It may use a remote (= slow) service.
// If instanceDigest is not nil, it contains a digest of the specific manifest instance to retrieve signatures for
// (when the primary manifest is a manifest list); this never happens if the primary manifest is not a manifest list
// (e.g. if the source never returns manifest lists).
func (s *ociArchiveImageSource) GetSignaturesWithFormat(ctx context.Context, instanceDigest *digest.Digest) ([]signature.Signature, error) {
	return s.unpackedSrc.GetSignaturesWithFormat(ctx, instanceDigest)
}

// LayerInfosForCopy returns either nil (meaning the values in the manifest are fine), or updated values for the layer
// blobsums that are listed in the image's manifest.  If values are returned, they should be used when using GetBlob()
// to read the image's layers.
// If instanceDigest is not nil, it contains a digest of the specific manifest instance to retrieve BlobInfos for
// (when the primary manifest is a manifest list); this never happens if the primary manifest is not a manifest list
// (e.g. if the source never returns manifest lists).
// The Digest field is guaranteed to be provided; Size may be -1.
// WARNING: The list may contain duplicates, and they are semantically relevant.
func (s *ociArchiveImageSource) LayerInfosForCopy(ctx context.Context, instanceDigest *digest.Digest) ([]types.BlobInfo, error) {
	return s.unpackedSrc.LayerInfosForCopy(ctx, instanceDigest)
}

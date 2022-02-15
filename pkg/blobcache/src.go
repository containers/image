package blobcache

import (
	"context"
	"io"
	"os"
	"sync"

	"github.com/containers/image/v5/internal/image"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/pkg/compression"
	"github.com/containers/image/v5/transports"
	"github.com/containers/image/v5/types"
	digest "github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	perrors "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type blobCacheSource struct {
	reference *BlobCache
	source    types.ImageSource
	sys       types.SystemContext
	// this mutex synchronizes the counters below
	mu          sync.Mutex
	cacheHits   int64
	cacheMisses int64
	cacheErrors int64
}

func (b *BlobCache) NewImageSource(ctx context.Context, sys *types.SystemContext) (types.ImageSource, error) {
	src, err := b.reference.NewImageSource(ctx, sys)
	if err != nil {
		return nil, perrors.Wrapf(err, "error creating new image source %q", transports.ImageName(b.reference))
	}
	logrus.Debugf("starting to read from image %q using blob cache in %q (compression=%v)", transports.ImageName(b.reference), b.directory, b.compress)
	return &blobCacheSource{reference: b, source: src, sys: *sys}, nil
}

func (s *blobCacheSource) Reference() types.ImageReference {
	return s.reference
}

func (s *blobCacheSource) Close() error {
	logrus.Debugf("finished reading from image %q using blob cache: cache had %d hits, %d misses, %d errors", transports.ImageName(s.reference), s.cacheHits, s.cacheMisses, s.cacheErrors)
	return s.source.Close()
}

func (s *blobCacheSource) GetManifest(ctx context.Context, instanceDigest *digest.Digest) ([]byte, string, error) {
	if instanceDigest != nil {
		filename := s.reference.blobPath(*instanceDigest, false)
		manifestBytes, err := os.ReadFile(filename)
		if err == nil {
			s.cacheHits++
			return manifestBytes, manifest.GuessMIMEType(manifestBytes), nil
		}
		if !os.IsNotExist(err) {
			s.cacheErrors++
			return nil, "", perrors.Wrap(err, "checking for manifest file")
		}
	}
	s.cacheMisses++
	return s.source.GetManifest(ctx, instanceDigest)
}

func (s *blobCacheSource) HasThreadSafeGetBlob() bool {
	return s.source.HasThreadSafeGetBlob()
}

func (s *blobCacheSource) GetBlob(ctx context.Context, blobinfo types.BlobInfo, cache types.BlobInfoCache) (io.ReadCloser, int64, error) {
	present, size, err := s.reference.HasBlob(blobinfo)
	if err != nil {
		return nil, -1, err
	}
	if present {
		for _, isConfig := range []bool{false, true} {
			filename := s.reference.blobPath(blobinfo.Digest, isConfig)
			f, err := os.Open(filename)
			if err == nil {
				s.mu.Lock()
				s.cacheHits++
				s.mu.Unlock()
				return f, size, nil
			}
			if !os.IsNotExist(err) {
				s.mu.Lock()
				s.cacheErrors++
				s.mu.Unlock()
				return nil, -1, perrors.Wrap(err, "checking for cache")
			}
		}
	}
	s.mu.Lock()
	s.cacheMisses++
	s.mu.Unlock()
	rc, size, err := s.source.GetBlob(ctx, blobinfo, cache)
	if err != nil {
		return rc, size, perrors.Wrapf(err, "error reading blob from source image %q", transports.ImageName(s.reference))
	}
	return rc, size, nil
}

func (s *blobCacheSource) GetSignatures(ctx context.Context, instanceDigest *digest.Digest) ([][]byte, error) {
	return s.source.GetSignatures(ctx, instanceDigest)
}

func (s *blobCacheSource) LayerInfosForCopy(ctx context.Context, instanceDigest *digest.Digest) ([]types.BlobInfo, error) {
	signatures, err := s.source.GetSignatures(ctx, instanceDigest)
	if err != nil {
		return nil, perrors.Wrapf(err, "error checking if image %q has signatures", transports.ImageName(s.reference))
	}
	canReplaceBlobs := !(len(signatures) > 0 && len(signatures[0]) > 0)

	infos, err := s.source.LayerInfosForCopy(ctx, instanceDigest)
	if err != nil {
		return nil, perrors.Wrapf(err, "error getting layer infos for copying image %q through cache", transports.ImageName(s.reference))
	}
	if infos == nil {
		img, err := image.FromUnparsedImage(ctx, &s.sys, image.UnparsedInstance(s.source, instanceDigest))
		if err != nil {
			return nil, perrors.Wrapf(err, "error opening image to get layer infos for copying image %q through cache", transports.ImageName(s.reference))
		}
		infos = img.LayerInfos()
	}

	if canReplaceBlobs && s.reference.compress != types.PreserveOriginal {
		replacedInfos := make([]types.BlobInfo, 0, len(infos))
		for _, info := range infos {
			var replaceDigest []byte
			var err error
			blobFile := s.reference.blobPath(info.Digest, false)
			alternate := ""
			switch s.reference.compress {
			case types.Compress:
				alternate = blobFile + compressedNote
				replaceDigest, err = os.ReadFile(alternate)
			case types.Decompress:
				alternate = blobFile + decompressedNote
				replaceDigest, err = os.ReadFile(alternate)
			}
			if err == nil && digest.Digest(replaceDigest).Validate() == nil {
				alternate = s.reference.blobPath(digest.Digest(replaceDigest), false)
				fileInfo, err := os.Stat(alternate)
				if err == nil {
					switch info.MediaType {
					case v1.MediaTypeImageLayer, v1.MediaTypeImageLayerGzip:
						switch s.reference.compress {
						case types.Compress:
							info.MediaType = v1.MediaTypeImageLayerGzip
							info.CompressionAlgorithm = &compression.Gzip
						case types.Decompress:
							info.MediaType = v1.MediaTypeImageLayer
							info.CompressionAlgorithm = nil
						}
					case manifest.DockerV2SchemaLayerMediaTypeUncompressed, manifest.DockerV2Schema2LayerMediaType:
						switch s.reference.compress {
						case types.Compress:
							info.MediaType = manifest.DockerV2Schema2LayerMediaType
							info.CompressionAlgorithm = &compression.Gzip
						case types.Decompress:
							// nope, not going to suggest anything, it's not allowed by the spec
							replacedInfos = append(replacedInfos, info)
							continue
						}
					}
					logrus.Debugf("suggesting cached blob with digest %q, type %q, and compression %v in place of blob with digest %q", string(replaceDigest), info.MediaType, s.reference.compress, info.Digest.String())
					info.CompressionOperation = s.reference.compress
					info.Digest = digest.Digest(replaceDigest)
					info.Size = fileInfo.Size()
					logrus.Debugf("info = %#v", info)
				}
			}
			replacedInfos = append(replacedInfos, info)
		}
		infos = replacedInfos
	}

	return infos, nil
}

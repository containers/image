package dist

import (
	"context"
	"fmt"
	"io"

	"github.com/containers/image/v5/types"
	"github.com/opencontainers/go-digest"
	ispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
)

// NOTE - the ImageSource interface is defined and commented in types.go

type distImageSource struct {
	s              *OciRepo
	ref            distReference
	manifest       *ispec.Manifest
	cachedManifest []byte
}

func (o *distImageSource) Reference() types.ImageReference {
	return o.ref
}

func (o *distImageSource) Close() error {
	return nil
}

func (o *distImageSource) GetManifest(ctx context.Context, instanceDigest *digest.Digest) ([]byte, string, error) {
	if instanceDigest != nil {
		return nil, "", fmt.Errorf("GetManifest with instanceDigest is not implemented")
	}

	if o.manifest == nil {
		bytes, m, err := o.s.GetManifest()
		if err != nil {
			return nil, "", errors.Wrap(err, "Failed fetching manifest")
		}

		o.cachedManifest = bytes
		o.manifest = m
	}

	return o.cachedManifest, ispec.MediaTypeImageManifest, nil
}

func (o *distImageSource) GetBlob(ctx context.Context, info types.BlobInfo,
	cache types.BlobInfoCache) (io.ReadCloser, int64, error) {
	digest := info.Digest.String()

	return o.s.GetLayer(digest)
}

func (o *distImageSource) HasThreadSafeGetBlob() bool {
	return true
}

func (o *distImageSource) GetSignatures(ctx context.Context, instanceDigest *digest.Digest) ([][]byte, error) {
	return [][]byte{}, nil
}

func (o *distImageSource) LayerInfosForCopy(ctx context.Context, layerDigest *digest.Digest) ([]types.BlobInfo, error) {
	return nil, nil
}

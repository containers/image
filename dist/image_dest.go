package dist

import (
	"context"
	"io"

	"github.com/containers/image/v5/pkg/blobinfocache/none"
	"github.com/containers/image/v5/types"
	"github.com/opencontainers/go-digest"
	ispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
)

// NOTE - the ImageDestination interface is defined in types.go

type distImageDest struct {
	s   *OciRepo
	ref distReference
}

func (o *distImageDest) Reference() types.ImageReference {
	return o.ref
}

func (o *distImageDest) Close() error {
	return nil
}

func (o *distImageDest) SupportedManifestMIMETypes() []string {
	return []string{
		ispec.MediaTypeImageManifest,
	}
}

func (o *distImageDest) SupportsSignatures(ctx context.Context) error {
	return nil
}

func (o *distImageDest) DesiredLayerCompression() types.LayerCompression {
	return types.PreserveOriginal
}

func (o *distImageDest) AcceptsForeignLayerURLs() bool {
	return true
}

func (o *distImageDest) MustMatchRuntimeOS() bool {
	return false
}

func (o *distImageDest) IgnoresEmbeddedDockerReference() bool {
	// Return value does not make a difference if Reference().DockerReference()
	// is nil.
	return true
}

// PutBlob writes contents of stream and returns data representing the result.
// inputInfo.Digest can be optionally provided if known; it is not mandatory for the implementation to verify it.
// inputInfo.Size is the expected length of stream, if known.
// inputInfo.MediaType describes the blob format, if known.
// May update cache.
// WARNING: The contents of stream are being verified on the fly.
// Until stream.Read() returns io.EOF, the contents of the data SHOULD NOT be available
// to any other readers for download using the supplied digest.
// If stream.Read() at any time, ESPECIALLY at end of input, returns an error, PutBlob MUST
// - 1) fail, and 2) delete any data stored so far.
func (o *distImageDest) PutBlob(ctx context.Context, stream io.Reader,
	inputInfo types.BlobInfo, cache types.BlobInfoCache, isConfig bool) (types.BlobInfo, error) {
	if inputInfo.Digest.String() != "" {
		ok, info, err := o.TryReusingBlob(ctx, inputInfo, none.NoCache, false)
		if err != nil {
			return types.BlobInfo{}, err
		}

		if ok {
			return info, nil
		}
	}

	// Do this as a chunked upload so we can calculate the digest, since
	// caller is not giving it to us.
	u, err := o.s.StartLayer()

	if err != nil {
		return types.BlobInfo{}, err
	}

	digest, size, err := o.s.CompleteLayer(u, stream)

	return types.BlobInfo{Digest: digest, Size: size}, err
}

// HasThreadSafePutBlob indicates whether PutBlob can be executed concurrently.
func (o *distImageDest) HasThreadSafePutBlob() bool {
	return true
}

func (o *distImageDest) TryReusingBlob(ctx context.Context, info types.BlobInfo,
	cache types.BlobInfoCache, canSubstitute bool) (bool, types.BlobInfo, error) {
	if info.Digest == "" {
		return false, types.BlobInfo{}, errors.Errorf(`"Can not check for a blob with unknown digest`)
	}

	if o.s.HasLayer(info.Digest.String()) {
		return true, types.BlobInfo{Digest: info.Digest, Size: -1}, nil
	}

	return false, types.BlobInfo{}, nil
}

func (o *distImageDest) PutManifest(ctx context.Context, m []byte, d *digest.Digest) error {
	return o.s.PutManifest(m)
}

func (o *distImageDest) PutSignatures(ctx context.Context, signatures [][]byte, d *digest.Digest) error {
	return nil
}

func (o *distImageDest) Commit(ctx context.Context, image types.UnparsedImage) error {
	return nil
}

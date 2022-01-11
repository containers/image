package impl

import (
	"context"
	"io"

	"github.com/containers/image/v5/internal/private"
	"github.com/containers/image/v5/types"
)

// Compat implements the obsolete parts of types.ImageDestination
// for implementations of private.ImageDestination.
// See AddCompat below.
type Compat struct {
	dest private.ImageDestinationInternalOnly
}

// AddCompat initializes Compat to implement the obsolete parts of types.ImageDestination
// for implementations of private.ImageDestination.
//
// Use it like this:
// type yourDestination struct {
//     impl.Compat
//     …
// }
// dest := &yourDestination{…}
// dest.Compat = impl.AddCompat(dest)
//
func AddCompat(dest private.ImageDestinationInternalOnly) Compat {
	return Compat{dest}
}

// PutBlob writes contents of stream and returns data representing the result.
// inputInfo.Digest can be optionally provided if known; if provided, and stream is read to the end without error, the digest MUST match the stream contents.
// inputInfo.Size is the expected length of stream, if known.
// inputInfo.MediaType describes the blob format, if known.
// May update cache.
// WARNING: The contents of stream are being verified on the fly.  Until stream.Read() returns io.EOF, the contents of the data SHOULD NOT be available
// to any other readers for download using the supplied digest.
// If stream.Read() at any time, ESPECIALLY at end of input, returns an error, PutBlob MUST 1) fail, and 2) delete any data stored so far.
func (c *Compat) PutBlob(ctx context.Context, stream io.Reader, inputInfo types.BlobInfo, cache types.BlobInfoCache, isConfig bool) (types.BlobInfo, error) {
	return c.dest.PutBlobWithOptions(ctx, stream, inputInfo, private.PutBlobOptions{
		Cache:    cache,
		IsConfig: isConfig,
	})
}

// TryReusingBlob checks whether the transport already contains, or can efficiently reuse, a blob, and if so, applies it to the current destination
// (e.g. if the blob is a filesystem layer, this signifies that the changes it describes need to be applied again when composing a filesystem tree).
// info.Digest must not be empty.
// If canSubstitute, TryReusingBlob can use an equivalent equivalent of the desired blob; in that case the returned info may not match the input.
// If the blob has been successfully reused, returns (true, info, nil); info must contain at least a digest and size, and may
// include CompressionOperation and CompressionAlgorithm fields to indicate that a change to the compression type should be
// reflected in the manifest that will be written.
// If the transport can not reuse the requested blob, TryReusingBlob returns (false, {}, nil); it returns a non-nil error only on an unexpected failure.
// May use and/or update cache.
func (c *Compat) TryReusingBlob(ctx context.Context, info types.BlobInfo, cache types.BlobInfoCache, canSubstitute bool) (bool, types.BlobInfo, error) {
	return c.dest.TryReusingBlobWithOptions(ctx, info, private.TryReusingBlobOptions{
		Cache:         cache,
		CanSubstitute: canSubstitute,
	})
}

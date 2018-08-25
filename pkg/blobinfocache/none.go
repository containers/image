package blobinfocache

import (
	"github.com/containers/image/types"
	"github.com/opencontainers/go-digest"
)

// noCache implements a dummy BlobInfoCache which records no data.
type noCache struct {
}

// NoCache implements BlobInfoCache by not recording any data.
//
// This exists primarily for implementations of configGetter for Manifest.Inspect,
// because configs only have one representation.
// Any use of BlobInfoCache with blobs should usually use at least a short-lived cache.
var NoCache types.BlobInfoCache = noCache{}

func (noCache) UncompressedDigest(anyDigest digest.Digest) digest.Digest {
	return ""
}

func (noCache) RecordUncompressedDigest(compressed digest.Digest, uncompressed digest.Digest) {
}

func (noCache) KnownLocations(transport types.ImageTransport, scope types.BICTransportScope, blobDigest digest.Digest) []types.BICLocationReference {
	return nil
}

func (noCache) RecordKnownLocation(transport types.ImageTransport, scope types.BICTransportScope, blobDigest digest.Digest, location types.BICLocationReference) {
}

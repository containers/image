package blobinfocache

import (
	"github.com/containers/image/types"
	"github.com/opencontainers/go-digest"
)

// LocationReference FIXME transport-dependent contents
type LocationReference struct{}

// TransportScope FIXME transport-dependent contents
type TransportScope struct{}

// BlobInfoCache FIXME do we need an interface or just global functions with a types.SystemContext?
type BlobInfoCache interface {
	UncompressedDigest(anyDigest digest.Digest) digest.Digest
	RecordUncompressedDigest(compressed digest.Digest, uncompressed digest.Digest)

	KnownLocations(transport types.ImageTransport, scope TransportScope, digest digest.Digest) []LocationReference
	RecordKnownLocation(transport types.ImageTransport, scope TransportScope, digest digest.Digest, location LocationReference)
}

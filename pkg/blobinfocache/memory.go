package blobinfocache

import (
	"github.com/containers/image/types"
	"github.com/opencontainers/go-digest"
	"github.com/sirupsen/logrus"
)

// locationKey only exists to make lookup in knownLocations easier.
type locationKey struct {
	transport  string
	scope      types.BICTransportScope
	blobDigest digest.Digest
}

// memoryCache implements an in-memory-only BlobInfoCache
type memoryCache struct {
	uncompressedDigests map[digest.Digest]digest.Digest
	knownLocations      map[locationKey][]types.BICLocationReference
}

// NewMemoryCache returns a BlobInfoCache implementation which is in-memory only.
// This is ONLY intended for tests. (FIXME: or to opt out of caching? How?)
// FIXME: Move it into an internal subpackage?
func NewMemoryCache() types.BlobInfoCache {
	return &memoryCache{
		uncompressedDigests: map[digest.Digest]digest.Digest{},
		knownLocations:      map[locationKey][]types.BICLocationReference{},
	}
}

func (mem *memoryCache) UncompressedDigest(anyDigest digest.Digest) digest.Digest {
	return mem.uncompressedDigests[anyDigest] // "" if not present in the map
}

func (mem *memoryCache) RecordUncompressedDigest(compressed digest.Digest, uncompressed digest.Digest) {
	if previous, ok := mem.uncompressedDigests[compressed]; ok && previous != uncompressed {
		logrus.Warnf("Uncompressed digest for blob %s previously recorded as %s, now %s", compressed, previous, uncompressed)
	}
	mem.uncompressedDigests[compressed] = uncompressed
}

func (mem *memoryCache) KnownLocations(transport types.ImageTransport, scope types.BICTransportScope, blobDigest digest.Digest) []types.BICLocationReference {
	return mem.knownLocations[locationKey{transport: transport.Name(), scope: scope, blobDigest: blobDigest}] // nil if not present
}

func (mem *memoryCache) RecordKnownLocation(transport types.ImageTransport, scope types.BICTransportScope, blobDigest digest.Digest, location types.BICLocationReference) {
	key := locationKey{transport: transport.Name(), scope: scope, blobDigest: blobDigest}
	old := mem.knownLocations[key] // nil if not present
	for _, l := range old {
		if l == location { // FIXME? Need an equality comparison for the abstract reference types.
			return
		}
	}
	mem.knownLocations[key] = append(old, location)
}

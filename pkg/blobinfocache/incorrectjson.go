package blobinfocache

import (
	"encoding/json"
	"io/ioutil"

	"github.com/containers/image/types"
	"github.com/opencontainers/go-digest"
	"github.com/sirupsen/logrus"
)

type incorrectJSON struct {
	path string
}

// FIXME: For CRI-O, does this need to hide information between different users?
type savedJSON struct {
	UncompressedDigests map[digest.Digest]digest.Digest
	KnownLocations      map[string]map[string]map[digest.Digest][]types.BICLocationReference
}

// NewIncorrectJSONCache FIXME
func NewIncorrectJSONCache() types.BlobInfoCache {
	return &incorrectJSON{
		path: "./incorrect-json-blobinfocache", // FIXME
	}
}

// emptyData returns a valid empty savedJSOn instance (notably with non-nil maps)
func emptyData() savedJSON {
	return savedJSON{
		UncompressedDigests: map[digest.Digest]digest.Digest{},
		KnownLocations:      map[string]map[string]map[digest.Digest][]types.BICLocationReference{},
	}
}

// ignores errors, returns empty data.
func (i *incorrectJSON) load() savedJSON {
	data, err := ioutil.ReadFile(i.path)
	if err != nil {
		logrus.Debugf("incorrectJSON loading failed: %v", err)
		return emptyData()
	}
	res := emptyData()
	if err := json.Unmarshal(data, &res); err != nil {
		logrus.Debugf("Internal error: incorrectJSON unmarshaling failed: %v", err)
		return emptyData()
	}
	return res
}

// ignores errors
func (i *incorrectJSON) save(data savedJSON) {
	bytes, err := json.Marshal(data)
	if err != nil {
		logrus.Debugf("Internal error: IncorrectJSON marshalling failed: %v", err)
		return
	}
	if err := ioutil.WriteFile(i.path, bytes, 0600); err != nil {
		logrus.Debugf("incorrectJSON saving failed: %v", err)
		return
	}
}

func (i *incorrectJSON) UncompressedDigest(anyDigest digest.Digest) digest.Digest {
	data := i.load()
	return data.UncompressedDigests[anyDigest] // "" if not present in the map
}

func (i *incorrectJSON) RecordUncompressedDigest(compressed digest.Digest, uncompressed digest.Digest) {
	data := i.load()
	if previous, ok := data.UncompressedDigests[compressed]; ok && previous != uncompressed {
		logrus.Warnf("Uncompressed digest for blob %s previously recorded as %s, now %s", compressed, previous, uncompressed)
	}
	data.UncompressedDigests[compressed] = uncompressed
	i.save(data)
}

func (i *incorrectJSON) KnownLocations(transport types.ImageTransport, scope types.BICTransportScope, blobDigest digest.Digest) []types.BICLocationReference {
	data := i.load()
	return data.KnownLocations[transport.Name()][scope.Opaque][blobDigest] // "" if not present in any of the the maps
}

func (i *incorrectJSON) RecordKnownLocation(transport types.ImageTransport, scope types.BICTransportScope, blobDigest digest.Digest, location types.BICLocationReference) {
	data := i.load()

	// FIXME? This is ridiculous. We might prefer a single struct key, but that can't be represented in JSON.
	if _, ok := data.KnownLocations[transport.Name()]; !ok {
		data.KnownLocations[transport.Name()] = map[string]map[digest.Digest][]types.BICLocationReference{}
	}
	if _, ok := data.KnownLocations[transport.Name()][scope.Opaque]; !ok {
		data.KnownLocations[transport.Name()][scope.Opaque] = map[digest.Digest][]types.BICLocationReference{}
	}

	old := data.KnownLocations[transport.Name()][scope.Opaque][blobDigest] // nil if not present
	for _, l := range old {
		if l == location { // FIXME? Need an equality comparison for the abstract reference types.
			return
		}
	}
	data.KnownLocations[transport.Name()][scope.Opaque][blobDigest] = append(old, location)

	i.save(data)
}

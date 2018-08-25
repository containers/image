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

type savedJSON struct {
	UncompressedDigests map[digest.Digest]digest.Digest
	KnownLocations      map[string]map[types.BICTransportScope]map[digest.Digest][]types.BICLocationReference
}

// NewIncorrectJSONCache FIXME
func NewIncorrectJSONCache() types.BlobInfoCache {
	return &incorrectJSON{
		path: "./incorrect-json-blobinfocache", // FIXME
	}
}

// ignores errors, returns empty data.
func (i *incorrectJSON) load() savedJSON {
	data, err := ioutil.ReadFile(i.path)
	if err != nil {
		return savedJSON{}
	}
	res := savedJSON{}
	if err := json.Unmarshal(data, &res); err != nil {
		return savedJSON{}
	}
	return res
}

// ignores errors
func (i *incorrectJSON) save(data savedJSON) {
	bytes, err := json.Marshal(data)
	if err != nil {
		return
	}
	if err := ioutil.WriteFile(i.path, bytes, 0600); err != nil {
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
	return data.KnownLocations[transport.Name()][scope][blobDigest] // "" if not present in any of the the maps
}

func (i *incorrectJSON) RecordKnownLocation(transport types.ImageTransport, scope types.BICTransportScope, blobDigest digest.Digest, location types.BICLocationReference) {
	data := i.load()

	// FIXME? This is ridiculous. We might prefer a single struct key, but that can't be represented in JSON.
	if _, ok := data.KnownLocations[transport.Name()]; !ok {
		data.KnownLocations[transport.Name()] = map[types.BICTransportScope]map[digest.Digest][]types.BICLocationReference{}
	}
	if _, ok := data.KnownLocations[transport.Name()][scope]; !ok {
		data.KnownLocations[transport.Name()][scope] = map[digest.Digest][]types.BICLocationReference{}
	}

	old := data.KnownLocations[transport.Name()][scope][blobDigest] // nil if not present
	for _, l := range old {
		if l == location { // FIXME? Need an equality comparison for the abstract reference types.
			return
		}
	}
	data.KnownLocations[transport.Name()][scope][blobDigest] = append(old, location)

	i.save(data)
}

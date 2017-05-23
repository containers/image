package signatures

import (
	"fmt"
	"path/filepath"

	"github.com/containers/image/docker/reference"
)

const (
	// fxConfigDigest is the config digest used within the fixture store.
	fxConfigDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	// fxManifestDigest is the digest of the manifest used within the fixture store.
	fxManifestDigest = "sha256:426c390db1e8405a6aa1de0d5abf0ad29146740bc5319fd3b323a0681789e47b"
)

// fxNamedTaggedReference is the named reference used within the fixture store.
var fxNamedTaggedReference reference.NamedTagged

func init() {
	ref, err := reference.ParseNormalizedNamed("docker.io/library/busybox:latest")
	if err != nil {
		panic(fmt.Sprintf("%#v", err))
	}
	nt, ok := ref.(reference.NamedTagged)
	if !ok {
		panic("ref is not NamedTagged")
	}
	fxNamedTaggedReference = nt
}

// fxManifestContents is the expected contents of the manifest with fxManifestDigest.
// This is not really a reasonable manifest, just enough for Store.RecordImage to be able to work with it.
var fxManifestContents = []byte(`{"mediaType":"application/vnd.docker.distribution.manifest.v2+json","config":{"digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}`)

// fxTagMappingKey is the key corresponding to (fxConfigDigest, fxNamedTaggedReference)
var fxTagMappingKey = []byte("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\x00docker.io/library/busybox:latest")

// fxDataKey value is the key corresponding to (fxConfigDigest, fxManifestDigest)
var fxDataKey = []byte("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\x00sha256:426c390db1e8405a6aa1de0d5abf0ad29146740bc5319fd3b323a0681789e47b")

// fxSignatures is the expected set of signatures for fxConfigDigest/fxManifestDigest
var fxSignatures = [][]byte{[]byte("signature 1"), []byte("signature 2"), []byte("signature 3")}

// fxDBPath is the path to the fixture store.
// NOTE: The fixture has been created on a little-endian system, and tests which rely on it are expected to fail on big-endian ones.
var fxDBPath = filepath.Join("testdata", "signatures.db")

// fxDBDumpPath is the path to dumpBoltDB output for TextFixtureDB
var fxDBDumpPath = filepath.Join("testdata", "signatures.db-dump")

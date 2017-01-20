package reference

import (

	// "opencontainers/go-digest" requires us to load the algorithms that we
	// want to use into the binary (it calls .Available).
	_ "crypto/sha256"

	distreference "github.com/docker/distribution/reference"
	"github.com/opencontainers/go-digest"
	"github.com/pkg/errors"
)

// XParseIDOrReference parses string for an image ID or a reference. ID can be
// without a default prefix.
func XParseIDOrReference(idOrRef string) (digest.Digest, distreference.Named, error) {
	ref, err := distreference.ParseAnyReference(idOrRef)
	if err != nil {
		return "", nil, err
	}
	if named, ok := ref.(distreference.Named); ok {
		return "", named, nil
	}
	if digested, ok := ref.(distreference.Digested); ok {
		return digest.Digest(digested.Digest()), nil, nil
	}
	return "", nil, errors.New("Unexpected inconsistency: %#v, the result of ParseAnyReference is neither a Named or nor a Digested")
}

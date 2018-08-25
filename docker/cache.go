package docker

import (
	"encoding/json"
	"fmt"

	"github.com/containers/image/docker/reference"
	"github.com/containers/image/types"
	digest "github.com/opencontainers/go-digest"
)

// bicTransportScope returns a BICTransportScope appropriate for ref.
func bicTransportScope(ref dockerReference) types.BICTransportScope {
	// Blobs can be reused across the whole registry.
	return types.BICTransportScope{Opaque: reference.Domain(ref.ref)}
}

// location is the value stored in BICLocationReference.
type bicLocation struct {
	Repository string
	Digest     digest.Digest
}

// newBICLocationReference returns a BICLocationReference appropriate for digest in ref.
func newBICLocationReference(ref dockerReference, digest digest.Digest) (types.BICLocationReference, error) {
	// Blobs are scoped to repositories (the tag/digest are not necessary to reuse a blob).
	// We also record the digest itself, to support returning a different but equivalent blob.
	loc := bicLocation{
		Repository: ref.ref.Name(),
		Digest:     digest,
	}
	s, err := json.Marshal(loc)
	if err != nil { // How can this ever happen?!
		return types.BICLocationReference{}, err
	}
	return types.BICLocationReference{Opaque: string(s)}, nil
}

// parseBICLocationReference returns a (repository, digest) for encoded lr.
func parseBICLocationReference(lr types.BICLocationReference) (reference.Named, digest.Digest, error) {
	loc := bicLocation{}
	if err := json.Unmarshal([]byte(lr.Opaque), &loc); err != nil {
		return nil, "", err
	}
	if loc.Repository == "" || loc.Digest == "" {
		return nil, "", fmt.Errorf("Internal error: docker bicLocation data missing in %q", lr.Opaque)
	}
	repo, err := reference.ParseNormalizedNamed(loc.Repository)
	if err != nil {
		return nil, "", err
	}
	if err := loc.Digest.Validate(); err != nil {
		return nil, "", err
	}
	return repo, loc.Digest, nil
}

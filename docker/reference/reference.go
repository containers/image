package reference

import (
	"regexp"

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
	if err := validateID(idOrRef); err == nil {
		idOrRef = "sha256:" + idOrRef
	}
	if dgst, err := digest.Parse(idOrRef); err == nil {
		return dgst, nil, nil
	}
	ref, err := distreference.ParseNormalizedNamed(idOrRef)
	return "", ref, err
}

var validHex = regexp.MustCompile(`^([a-f0-9]{64})$`)

func validateID(id string) error {
	if ok := validHex.MatchString(id); !ok {
		return errors.Errorf("image ID %q is invalid", id)
	}
	return nil
}

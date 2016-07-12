package docker

import (
	"fmt"

	"github.com/docker/docker/reference"
)

// parseImageName converts a string into a reference.
// It is guaranteed that reference.IsNameOnly is false for the returned value.
func parseImageName(img string) (reference.Named, error) {
	ref, err := reference.ParseNamed(img)
	if err != nil {
		return nil, err
	}
	ref = reference.WithDefaultTag(ref)
	if reference.IsNameOnly(ref) { // Sanity check that we are fulfulling our contract
		return nil, fmt.Errorf("Internal inconsistency: reference.IsNameOnly for reference %s (parsed from %s)", ref.String(), img)
	}
	return ref, nil
}

// tagOrDigest returns a tag or digest from a reference for which !reference.IsNameOnly.
func tagOrDigest(ref reference.Named) (string, error) {
	if ref, ok := ref.(reference.Canonical); ok {
		return ref.Digest().String(), nil
	}
	if ref, ok := ref.(reference.NamedTagged); ok {
		return ref.Tag(), nil
	}
	return "", fmt.Errorf("Internal inconsistency: Reference %s unexpectedly has neither a digest nor a tag", ref.String())
}

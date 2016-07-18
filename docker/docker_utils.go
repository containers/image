package docker

import (
	"fmt"

	"github.com/docker/docker/reference"
)

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

package docker

import (
	"net/url"
	"strings"

	"github.com/containers/image/v5/docker/reference"
	digest "github.com/opencontainers/go-digest"
)

func sigstoreSignatureURL(dstRef dockerReference, digest digest.Digest, scheme string) (*url.URL, error) {
	nameTagged, err := reference.WithTag(dstRef.ref, attachedImageTag(&digest))
	if err != nil {
		return nil, err
	}

	url, err := url.Parse(scheme + nameTagged.Name())
	if err != nil {
		return nil, err
	}

	return url, nil
}

func attachedImageTag(digest *digest.Digest) string {
	// sha256:d34db33f -> sha256-d34db33f.suffix
	return strings.ReplaceAll(digest.String(), ":", "-") + ".sig"
}

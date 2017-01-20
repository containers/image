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

const (
	// XDefaultTag defines the default tag used when performing images related actions and no tag or digest is specified
	XDefaultTag = "latest"
	// XDefaultHostname is the default built-in hostname
	XDefaultHostname = "docker.io"
	// XLegacyDefaultHostname is automatically converted to DefaultHostname
	XLegacyDefaultHostname = "index.docker.io"
	// XDefaultRepoPrefix is the prefix used for default repositories in default host
	XDefaultRepoPrefix = "library/"
)

// XParseNamed parses s and returns a syntactically valid reference implementing
// the Named interface. The reference must have a name, otherwise an error is
// returned.
// If an error was encountered it is returned, along with a nil Reference.
func XParseNamed(s string) (distreference.Named, error) {
	named, err := distreference.ParseNormalizedNamed(s)
	if err != nil {
		return nil, errors.Wrapf(err, "Error parsing reference: %q is not a valid repository/tag", s)
	}
	r, err := XWithName(named.Name())
	if err != nil {
		return nil, err
	}
	if canonical, isCanonical := named.(distreference.Canonical); isCanonical {
		return distreference.WithDigest(r, canonical.Digest())
	}
	if tagged, isTagged := named.(distreference.NamedTagged); isTagged {
		return distreference.WithTag(r, tagged.Tag())
	}
	return r, nil
}

// XWithName returns a named object representing the given string. If the input
// is invalid ErrReferenceInvalidFormat will be returned.
func XWithName(name string) (distreference.Named, error) {
	return distreference.ParseNormalizedNamed(name)
}

// XParseIDOrReference parses string for an image ID or a reference. ID can be
// without a default prefix.
func XParseIDOrReference(idOrRef string) (digest.Digest, distreference.Named, error) {
	if err := validateID(idOrRef); err == nil {
		idOrRef = "sha256:" + idOrRef
	}
	if dgst, err := digest.Parse(idOrRef); err == nil {
		return dgst, nil, nil
	}
	ref, err := XParseNamed(idOrRef)
	return "", ref, err
}

var validHex = regexp.MustCompile(`^([a-f0-9]{64})$`)

func validateID(id string) error {
	if ok := validHex.MatchString(id); !ok {
		return errors.Errorf("image ID %q is invalid", id)
	}
	return nil
}

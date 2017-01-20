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

// XCanonical reference is an object with a fully unique
// name including a name with hostname and digest
type XCanonical interface {
	distreference.Canonical
}

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
		r, err := distreference.WithDigest(r, canonical.Digest())
		if err != nil {
			return nil, err
		}
		return &canonicalRef{Canonical: r, namedRef: namedRef{r}}, nil
	}
	if tagged, isTagged := named.(distreference.NamedTagged); isTagged {
		return distreference.WithTag(r, tagged.Tag())
	}
	return r, nil
}

// XWithName returns a named object representing the given string. If the input
// is invalid ErrReferenceInvalidFormat will be returned.
// FIXME: returns *namedRef to expose the distreference.Named implementation. Should revert to distreference.Named.
func XWithName(name string) (*namedRef, error) {
	r, err := distreference.ParseNormalizedNamed(name)
	if err != nil {
		return nil, err
	}
	return &namedRef{r}, nil
}

type namedRef struct {
	distreference.Named // FIXME: must implement private distreference.NamedRepository
}
type canonicalRef struct {
	distreference.Canonical
	namedRef
}

// TEMPORARY: distreference.WithDigest and distreference.WithTag can work with any distreference.Named,
// but if so, they break the values of distreference.Domain() and distreference.Path(),
// and hence also distreference.FamiliarName()/distreference.FamiliarString().  To preserve this,
// we need to implement a PRIVATE distreference.namedRepository.
// Similarly, we need to implement a PRIVATE distreference.normalizedNamed so that distreference.Familiar*()
// knows how to compute the minimal form.
// Right now that happens by these REALLY UGLY methods; eventually we will eliminate namedRef entirely in favor of
// distreference.Named, and distreference can keep its implementation games to itself.
type drPRIVATEInterfaces interface {
	distreference.Named
	Domain() string
	Path() string
	Familiar() distreference.Named
}

func (r *namedRef) Domain() string {
	return r.Named.(drPRIVATEInterfaces).Domain()
}
func (r *namedRef) Path() string {
	return r.Named.(drPRIVATEInterfaces).Path()
}
func (r *namedRef) Familiar() distreference.Named {
	return r.Named.(drPRIVATEInterfaces).Familiar()
}

// XWithDefaultTag adds a default tag to a reference if it only has a repo name.
func XWithDefaultTag(ref distreference.Named) distreference.Named {
	if XIsNameOnly(ref) {
		ref, _ = distreference.WithTag(ref, XDefaultTag)
	}
	return ref
}

// XIsNameOnly returns true if reference only contains a repo name.
func XIsNameOnly(ref distreference.Named) bool {
	if _, ok := ref.(distreference.NamedTagged); ok {
		return false
	}
	if _, ok := ref.(XCanonical); ok {
		return false
	}
	return true
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

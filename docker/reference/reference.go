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

// XNamed is an object with a full name
type XNamed interface {
	distreference.Named
}

// XNamedTagged is an object including a name and tag.
type XNamedTagged interface {
	XNamed
	XTag() string
}

// XCanonical reference is an object with a fully unique
// name including a name with hostname and digest
type XCanonical interface {
	XNamed
	XDigest() digest.Digest
}

// XParseNamed parses s and returns a syntactically valid reference implementing
// the Named interface. The reference must have a name, otherwise an error is
// returned.
// If an error was encountered it is returned, along with a nil Reference.
func XParseNamed(s string) (XNamed, error) {
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
		return &canonicalRef{namedRef{r}}, nil
	}
	if tagged, isTagged := named.(distreference.NamedTagged); isTagged {
		return XWithTag(r, tagged.Tag())
	}
	return r, nil
}

// XWithName returns a named object representing the given string. If the input
// is invalid ErrReferenceInvalidFormat will be returned.
// FIXME: returns *namedRef to expose the distreference.Named implementation. Should revert to XNamed/Named.
func XWithName(name string) (*namedRef, error) {
	r, err := distreference.ParseNormalizedNamed(name)
	if err != nil {
		return nil, err
	}
	return &namedRef{r}, nil
}

// XWithTag combines the name from "name" and the tag from "tag" to form a
// reference incorporating both the name and the tag.
// FIXME: expects *namedRef to expose the distreference.Named implementation. Should revert to XNamed/Named.
func XWithTag(name *namedRef, tag string) (XNamedTagged, error) {
	r, err := distreference.WithTag(name, tag)
	if err != nil {
		return nil, err
	}
	return &taggedRef{namedRef{r}}, nil
}

type namedRef struct {
	distreference.Named // FIXME: must implement private distreference.NamedRepository
}
type taggedRef struct {
	namedRef
}
type canonicalRef struct {
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

func (r *taggedRef) XTag() string {
	return r.namedRef.Named.(distreference.NamedTagged).Tag()
}
func (r *canonicalRef) XDigest() digest.Digest {
	return digest.Digest(r.namedRef.Named.(distreference.Canonical).Digest())
}

// XWithDefaultTag adds a default tag to a reference if it only has a repo name.
func XWithDefaultTag(ref XNamed) XNamed {
	if XIsNameOnly(ref) {
		// FIXME: uses *namedRef to expose the distreference.Named implementations. Should use ref without a cast.
		ref, _ = XWithTag(ref.(*namedRef), XDefaultTag)
	}
	return ref
}

// XIsNameOnly returns true if reference only contains a repo name.
func XIsNameOnly(ref XNamed) bool {
	if _, ok := ref.(XNamedTagged); ok {
		return false
	}
	if _, ok := ref.(XCanonical); ok {
		return false
	}
	return true
}

// XParseIDOrReference parses string for an image ID or a reference. ID can be
// without a default prefix.
func XParseIDOrReference(idOrRef string) (digest.Digest, XNamed, error) {
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

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
	// XName returns normalized repository name, like "ubuntu".
	XName() string
	// XString returns full reference, like "ubuntu@sha256:abcdef..."
	XString() string
	// XFullName returns full repository name with hostname, like "docker.io/library/ubuntu"
	XFullName() string
	// XHostname returns hostname for the reference, like "docker.io"
	XHostname() string
	// XRemoteName returns the repository component of the full name, like "library/ubuntu"
	XRemoteName() string
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
		r, err := distreference.WithDigest(r.upstream, canonical.Digest())
		if err != nil {
			return nil, err
		}
		return &canonicalRef{namedRef{upstream: r}}, nil
	}
	if tagged, isTagged := named.(distreference.NamedTagged); isTagged {
		return XWithTag(r, tagged.Tag())
	}
	return r, nil
}

// XWithName returns a named object representing the given string. If the input
// is invalid ErrReferenceInvalidFormat will be returned.
// FIXME: returns *namedRef to expose the upstream field. Should revert to XNamed/Named.
func XWithName(name string) (*namedRef, error) {
	r, err := distreference.ParseNormalizedNamed(name)
	if err != nil {
		return nil, err
	}
	return &namedRef{upstream: r}, nil
}

// XWithTag combines the name from "name" and the tag from "tag" to form a
// reference incorporating both the name and the tag.
// FIXME: expects *namedRef to expose the upstream field. Should revert to XNamed/Named.
func XWithTag(name *namedRef, tag string) (XNamedTagged, error) {
	r, err := distreference.WithTag(name.upstream, tag)
	if err != nil {
		return nil, err
	}
	return &taggedRef{namedRef{upstream: r}}, nil
}

type namedRef struct {
	// upstream uses the normalization from distreference.ParseNormalizedNamed
	upstream distreference.Named
}
type taggedRef struct {
	namedRef
}
type canonicalRef struct {
	namedRef
}

func (r *namedRef) XName() string {
	return distreference.FamiliarName(r.upstream)
}
func (r *namedRef) XString() string {
	return distreference.FamiliarString(r.upstream)
}
func (r *namedRef) XFullName() string {
	return r.upstream.Name()
}
func (r *namedRef) XHostname() string {
	return distreference.Domain(r.upstream)
}
func (r *namedRef) XRemoteName() string {
	return distreference.Path(r.upstream)
}
func (r *taggedRef) XTag() string {
	return r.namedRef.upstream.(distreference.NamedTagged).Tag()
}
func (r *canonicalRef) XDigest() digest.Digest {
	return digest.Digest(r.namedRef.upstream.(distreference.Canonical).Digest())
}

// XWithDefaultTag adds a default tag to a reference if it only has a repo name.
func XWithDefaultTag(ref XNamed) XNamed {
	if XIsNameOnly(ref) {
		// FIXME: uses *namedRef to expose the upstream fields. Should use ref without a cast.
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

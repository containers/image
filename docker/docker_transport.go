package docker

import (
	"fmt"
	"strings"

	"github.com/containers/image/types"
	"github.com/docker/docker/reference"
)

// Transport is an ImageTransport for Docker references.
var Transport = dockerTransport{}

type dockerTransport struct{}

func (t dockerTransport) Name() string {
	return "docker"
}

// ParseReference converts a string, which should not start with the ImageTransport.Name prefix, into an ImageReference.
func (t dockerTransport) ParseReference(reference string) (types.ImageReference, error) {
	return ParseReference(reference)
}

// dockerReference is an ImageReference for Docker images.
type dockerReference struct {
	ref reference.Named // By construction we know that !reference.IsNameOnly(ref)
}

// ParseReference converts a string, which should not start with the ImageTransport.Name prefix, into an Docker ImageReference.
func ParseReference(refString string) (types.ImageReference, error) {
	if !strings.HasPrefix(refString, "//") {
		return nil, fmt.Errorf("docker: image reference %s does not start with //", refString)
	}
	ref, err := reference.ParseNamed(strings.TrimPrefix(refString, "//"))
	if err != nil {
		return nil, err
	}
	ref = reference.WithDefaultTag(ref)
	return NewReference(ref)
}

// NewReference returns a Docker reference for a named reference. The reference must satisfy !reference.IsNameOnly().
func NewReference(ref reference.Named) (types.ImageReference, error) {
	if reference.IsNameOnly(ref) {
		return nil, fmt.Errorf("Docker reference %s has neither a tag nor a digest", ref.String())
	}
	// A github.com/distribution/reference value can have a tag and a digest at the same time!
	// docker/reference does not handle that, so fail.
	// (Even if it were supported, the semantics of policy namespaces are unclear - should we drop
	// the tag or the digest first?)
	_, isTagged := ref.(reference.NamedTagged)
	_, isDigested := ref.(reference.Canonical)
	if isTagged && isDigested {
		return nil, fmt.Errorf("Docker references with both a tag and digest are currently not supported")
	}
	return dockerReference{
		ref: ref,
	}, nil
}

func (ref dockerReference) Transport() types.ImageTransport {
	return Transport
}

// StringWithinTransport returns a string representation of the reference, which MUST be such that
// reference.Transport().ParseReference(reference.StringWithinTransport()) returns an equivalent reference.
// NOTE: The returned string is not promised to be equal to the original input to ParseReference;
// e.g. default attribute values omitted by the user may be filled in in the return value, or vice versa.
// WARNING: Do not use the return value in the UI to describe an image, it does not contain the Transport().Name() prefix.
func (ref dockerReference) StringWithinTransport() string {
	return "//" + ref.ref.String()
}

// DockerReference returns a Docker reference associated with this reference
// (fully explicit, i.e. !reference.IsNameOnly, but reflecting user intent,
// not e.g. after redirect or alias processing), or nil if unknown/not applicable.
func (ref dockerReference) DockerReference() reference.Named {
	return ref.ref
}

// NewImage returns a types.Image for this reference.
func (ref dockerReference) NewImage(certPath string, tlsVerify bool) (types.Image, error) {
	return newImage(ref, certPath, tlsVerify)
}

// NewImageSource returns a types.ImageSource for this reference.
func (ref dockerReference) NewImageSource(certPath string, tlsVerify bool) (types.ImageSource, error) {
	return newImageSource(ref, certPath, tlsVerify)
}

// NewImageDestination returns a types.ImageDestination for this reference.
func (ref dockerReference) NewImageDestination(certPath string, tlsVerify bool) (types.ImageDestination, error) {
	return newImageDestination(ref, certPath, tlsVerify)
}

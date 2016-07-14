package directory

import (
	"github.com/containers/image/image"
	"github.com/containers/image/types"
	"github.com/docker/docker/reference"
)

// Transport is an ImageTransport for directory paths.
var Transport = dirTransport{}

type dirTransport struct{}

func (t dirTransport) Name() string {
	return "dir"
}

// ParseReference converts a string, which should not start with the ImageTransport.Name prefix, into an ImageReference.
func (t dirTransport) ParseReference(reference string) (types.ImageReference, error) {
	return NewReference(reference), nil
}

// dirReference is an ImageReference for directory paths.
type dirReference struct {
	// Note that the interpretation of paths below depends on the underlying filesystem state, which may change under us at any time!
	path string // As specified by the user. May be relative, contain symlinks, etc.
}

// There is no directory.ParseReference because it is rather pointless.
// Callers who need a transport-independent interface will go through
// dirTransport.ParseReference; callers who intentionally deal with directories
// can use directory.NewReference.

// NewReference returns a directory reference for a specified path.
func NewReference(path string) types.ImageReference {
	return dirReference{path: path}
}

func (ref dirReference) Transport() types.ImageTransport {
	return Transport
}

// StringWithinTransport returns a string representation of the reference, which MUST be such that
// reference.Transport().ParseReference(reference.StringWithinTransport()) returns an equivalent reference.
// NOTE: The returned string is not promised to be equal to the original input to ParseReference;
// e.g. default attribute values omitted by the user may be filled in in the return value, or vice versa.
// WARNING: Do not use the return value in the UI to describe an image, it does not contain the Transport().Name() prefix.
func (ref dirReference) StringWithinTransport() string {
	return ref.path
}

// DockerReference returns a Docker reference associated with this reference
// (fully explicit, i.e. !reference.IsNameOnly, but reflecting user intent,
// not e.g. after redirect or alias processing), or nil if unknown/not applicable.
func (ref dirReference) DockerReference() reference.Named {
	return nil
}

// PolicyConfigurationIdentity returns a string representation of the reference, suitable for policy lookup.
// This MUST reflect user intent, not e.g. after processing of third-party redirects or aliases;
// The value SHOULD be fully explicit about its semantics, with no hidden defaults, AND canonical
// (i.e. various references with exactly the same semantics should return the same configuration identity)
// It is fine for the return value to be equal to StringWithinTransport(), and it is desirable but
// not required/guaranteed that it will be a valid input to Transport().ParseReference().
// Returns "" if configuration identities for these references are not supported.
func (ref dirReference) PolicyConfigurationIdentity() string {
	return ""
}

// PolicyConfigurationNamespaces returns a list of other policy configuration namespaces to search
// for if explicit configuration for PolicyConfigurationIdentity() is not set.  The list will be processed
// in order, terminating on first match, and an implicit "" is always checked at the end.
// It is STRONGLY recommended for the first element, if any, to be a prefix of PolicyConfigurationIdentity(),
// and each following element to be a prefix of the element preceding it.
func (ref dirReference) PolicyConfigurationNamespaces() []string {
	return nil
}

// NewImage returns a types.Image for this reference.
func (ref dirReference) NewImage(certPath string, tlsVerify bool) (types.Image, error) {
	src := newImageSource(ref)
	return image.FromSource(src, nil), nil
}

// NewImageSource returns a types.ImageSource for this reference.
func (ref dirReference) NewImageSource(certPath string, tlsVerify bool) (types.ImageSource, error) {
	return newImageSource(ref), nil
}

// NewImageDestination returns a types.ImageDestination for this reference.
func (ref dirReference) NewImageDestination(certPath string, tlsVerify bool) (types.ImageDestination, error) {
	return newImageDestination(ref), nil
}

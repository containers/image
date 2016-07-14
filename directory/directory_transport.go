package directory

import (
	"github.com/containers/image/image"
	"github.com/containers/image/types"
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

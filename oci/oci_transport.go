package oci

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/containers/image/types"
)

// Transport is an ImageTransport for Docker references.
var Transport = ociTransport{}

type ociTransport struct{}

func (t ociTransport) Name() string {
	return "oci"
}

// ParseReference converts a string, which should not start with the ImageTransport.Name prefix, into an ImageReference.
func (t ociTransport) ParseReference(reference string) (types.ImageReference, error) {
	return ParseReference(reference)
}

// ociReference is an ImageReference for OCI directory paths.
type ociReference struct {
	// Note that the interpretation of paths below depends on the underlying filesystem state, which may change under us at any time!
	dir string // As specified by the user. May be relative, contain symlinks, etc.
	tag string
}

var refRegexp = regexp.MustCompile(`^([A-Za-z0-9._-]+)+$`)

// ParseReference converts a string, which should not start with the ImageTransport.Name prefix, into an OCI ImageReference.
func ParseReference(reference string) (types.ImageReference, error) {
	var dir, tag string
	sep := strings.LastIndex(reference, ":")
	if sep == -1 {
		dir = reference
		tag = "latest"
	} else {
		dir = reference[:sep]
		tag = reference[sep+1:]
		if !refRegexp.MatchString(tag) {
			return nil, fmt.Errorf("Invalid tag %s", tag)
		}
	}
	return NewReference(dir, tag), nil
}

// NewReference returns an OCI reference for a directory and a tag.
func NewReference(dir, tag string) types.ImageReference {
	return ociReference{dir: dir, tag: tag}
}

func (ref ociReference) Transport() types.ImageTransport {
	return Transport
}

// StringWithinTransport returns a string representation of the reference, which MUST be such that
// reference.Transport().ParseReference(reference.StringWithinTransport()) returns an equivalent reference.
// NOTE: The returned string is not promised to be equal to the original input to ParseReference;
// e.g. default attribute values omitted by the user may be filled in in the return value, or vice versa.
// WARNING: Do not use the return value in the UI to describe an image, it does not contain the Transport().Name() prefix.
func (ref ociReference) StringWithinTransport() string {
	return fmt.Sprintf("%s:%s", ref.dir, ref.tag)
}

// NewImage returns a types.Image for this reference.
func (ref ociReference) NewImage(certPath string, tlsVerify bool) (types.Image, error) {
	return nil, errors.New("Full Image support not implemented for oci: image names")
}

// NewImageSource returns a types.ImageSource for this reference.
func (ref ociReference) NewImageSource(certPath string, tlsVerify bool) (types.ImageSource, error) {
	return nil, errors.New("Reading images not implemented for oci: image names")
}

// NewImageDestination returns a types.ImageDestination for this reference.
func (ref ociReference) NewImageDestination(certPath string, tlsVerify bool) (types.ImageDestination, error) {
	return newImageDestination(ref), nil
}

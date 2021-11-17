// +build linux

package sifimage

import (
	"context"
	"fmt"
	"strings"

	"github.com/containers/image/v5/directory/explicitfilepath"
	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/image"
	"github.com/containers/image/v5/transports"
	"github.com/containers/image/v5/types"
	"github.com/pkg/errors"
)

func init() {
	transports.Register(Transport)
}

// Transport is an ImageTransport for SIF images.
var Transport = sifTransport{}

type sifTransport struct{}

// sifReference is an ImageReference for SIF images.
type sifReference struct {
	file         string // As specified by the user. May be relative, contain symlinks, etc.
	resolvedFile string // Absolute file path with no symlinks, at least at the time of its creation. Primarily used for policy namespaces.
}

func (t sifTransport) Name() string {
	return "sif"
}

// ParseReference converts a string, which should not start with the ImageTransport.Name prefix, into an ImageReference.
func (t sifTransport) ParseReference(reference string) (types.ImageReference, error) {
	return NewReference(reference)
}

// NewReference returns an image file reference for a specified path.
func NewReference(file string) (types.ImageReference, error) {
	resolved, err := explicitfilepath.ResolvePathToFullyExplicit(file)
	if err != nil {
		return nil, err
	}
	return sifReference{file: file, resolvedFile: resolved}, nil
}

// ValidatePolicyConfigurationScope checks that scope is a valid name for a signature.PolicyTransportScopes keys
// (i.e. a valid PolicyConfigurationIdentity() or PolicyConfigurationNamespaces() return value).
// It is acceptable to allow an invalid value which will never be matched, it can "only" cause user confusion.
// scope passed to this function will not be "", that value is always allowed.
func (t sifTransport) ValidatePolicyConfigurationScope(scope string) error {
	return errors.New(`sif: does not support any scopes except the default "" one`)
}

func (ref sifReference) Transport() types.ImageTransport {
	return Transport
}

// StringWithinTransport returns a string representation of the reference, which MUST be such that
// reference.Transport().ParseReference(reference.StringWithinTransport()) returns an equivalent reference.
func (ref sifReference) StringWithinTransport() string {
	return fmt.Sprintf("%s", ref.file)
}

// DockerReference returns a Docker reference associated with this reference
func (ref sifReference) DockerReference() reference.Named {
	return nil
}

// PolicyConfigurationIdentity returns a string representation of the reference, suitable for policy lookup.
func (ref sifReference) PolicyConfigurationIdentity() string {
	return ref.resolvedFile
}

// PolicyConfigurationNamespaces returns a list of other policy configuration namespaces to search
// for if explicit configuration for PolicyConfigurationIdentity() is not set
func (ref sifReference) PolicyConfigurationNamespaces() []string {
	res := []string{}
	path := ref.resolvedFile
	for {
		lastSlash := strings.LastIndex(path, "/")
		if lastSlash == -1 || path == "/" {
			break
		}
		res = append(res, path)
		path = path[:lastSlash]
	}
	return res
}

// NewImage returns a types.ImageCloser for this reference, possibly specialized for this ImageTransport.
// The caller must call .Close() on the returned ImageCloser.
// NOTE: If any kind of signature verification should happen, build an UnparsedImage from the value returned by NewImageSource,
// verify that UnparsedImage, and convert it into a real Image via image.FromUnparsedImage.
// WARNING: This may not do the right thing for a manifest list, see image.FromSource for details.
func (ref sifReference) NewImage(ctx context.Context, sys *types.SystemContext) (types.ImageCloser, error) {
	src, err := newImageSource(ctx, sys, ref)
	if err != nil {
		return nil, err
	}
	return image.FromSource(ctx, sys, src)
}

// NewImageSource returns a types.ImageSource for this reference.
// The caller must call .Close() on the returned ImageSource.
func (ref sifReference) NewImageSource(ctx context.Context, sys *types.SystemContext) (types.ImageSource, error) {
	return newImageSource(ctx, sys, ref)
}

func (ref sifReference) NewImageDestination(ctx context.Context, sys *types.SystemContext) (types.ImageDestination, error) {
	return nil, fmt.Errorf(`"sif:" locations can only be read from, not written to`)
}

// DeleteImage deletes the named image from the registry, if supported.
func (ref sifReference) DeleteImage(ctx context.Context, sys *types.SystemContext) error {
	return errors.Errorf("Deleting images not implemented for sif: images")
}

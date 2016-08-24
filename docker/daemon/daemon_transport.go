package daemon

import (
	"fmt"

	"github.com/containers/image/docker/reference"
	"github.com/containers/image/types"
)

// Transport is an ImageTransport for images managed by a local Docker daemon.
var Transport = daemonTransport{}

type daemonTransport struct{}

// Name returns the name of the transport, which must be unique among other transports.
func (t daemonTransport) Name() string {
	return "docker-daemon"
}

// ParseReference converts a string, which should not start with the ImageTransport.Name prefix, into an ImageReference.
func (t daemonTransport) ParseReference(reference string) (types.ImageReference, error) {
	return ParseReference(reference)
}

// ValidatePolicyConfigurationScope checks that scope is a valid name for a signature.PolicyTransportScopes keys
// (i.e. a valid PolicyConfigurationIdentity() or PolicyConfigurationNamespaces() return value).
// It is acceptable to allow an invalid value which will never be matched, it can "only" cause user confusion.
// scope passed to this function will not be "", that value is always allowed.
func (t daemonTransport) ValidatePolicyConfigurationScope(scope string) error {
	// FIXME FIXME
	return nil
}

// daemonReference is an ImageReference for images managed by a local Docker daemon.
type daemonReference string // FIXME FIXME

// ParseReference converts a string, which should not start with the ImageTransport.Name prefix, into an ImageReference.
func ParseReference(reference string) (types.ImageReference, error) {
	return daemonReference(reference), nil // FIXME FIXME
}

// FIXME FIXME: NewReference?

func (ref daemonReference) Transport() types.ImageTransport {
	return Transport
}

// StringWithinTransport returns a string representation of the reference, which MUST be such that
// reference.Transport().ParseReference(reference.StringWithinTransport()) returns an equivalent reference.
// NOTE: The returned string is not promised to be equal to the original input to ParseReference;
// e.g. default attribute values omitted by the user may be filled in in the return value, or vice versa.
// WARNING: Do not use the return value in the UI to describe an image, it does not contain the Transport().Name() prefix;
// instead, see transports.ImageName().
func (ref daemonReference) StringWithinTransport() string {
	return string(ref) // FIXME FIXME
}

// DockerReference returns a Docker reference associated with this reference
// (fully explicit, i.e. !reference.IsNameOnly, but reflecting user intent,
// not e.g. after redirect or alias processing), or nil if unknown/not applicable.
func (ref daemonReference) DockerReference() reference.Named {
	return nil // FIXME FIXME
}

// PolicyConfigurationIdentity returns a string representation of the reference, suitable for policy lookup.
// This MUST reflect user intent, not e.g. after processing of third-party redirects or aliases;
// The value SHOULD be fully explicit about its semantics, with no hidden defaults, AND canonical
// (i.e. various references with exactly the same semantics should return the same configuration identity)
// It is fine for the return value to be equal to StringWithinTransport(), and it is desirable but
// not required/guaranteed that it will be a valid input to Transport().ParseReference().
// Returns "" if configuration identities for these references are not supported.
func (ref daemonReference) PolicyConfigurationIdentity() string {
	return string(ref) // FIXME FIXME
}

// PolicyConfigurationNamespaces returns a list of other policy configuration namespaces to search
// for if explicit configuration for PolicyConfigurationIdentity() is not set.  The list will be processed
// in order, terminating on first match, and an implicit "" is always checked at the end.
// It is STRONGLY recommended for the first element, if any, to be a prefix of PolicyConfigurationIdentity(),
// and each following element to be a prefix of the element preceding it.
func (ref daemonReference) PolicyConfigurationNamespaces() []string {
	return []string{} // FIXME FIXME?
}

// NewImage returns a types.Image for this reference.
// The caller must call .Close() on the returned Image.
func (ref daemonReference) NewImage(ctx *types.SystemContext) (types.Image, error) {
	panic("FIXME FIXME")
}

// NewImageSource returns a types.ImageSource for this reference,
// asking the backend to use a manifest from requestedManifestMIMETypes if possible.
// nil requestedManifestMIMETypes means manifest.DefaultRequestedManifestMIMETypes.
// The caller must call .Close() on the returned ImageSource.
func (ref daemonReference) NewImageSource(ctx *types.SystemContext, requestedManifestMIMETypes []string) (types.ImageSource, error) {
	return newImageSource(ctx, ref)
}

// NewImageDestination returns a types.ImageDestination for this reference.
// The caller must call .Close() on the returned ImageDestination.
func (ref daemonReference) NewImageDestination(ctx *types.SystemContext) (types.ImageDestination, error) {
	return newImageDestination(ctx, ref)
}

// DeleteImage deletes the named image from the registry, if supported.
func (ref daemonReference) DeleteImage(ctx *types.SystemContext) error {
	return fmt.Errorf("Deleting images not implemented for docker-daemon: images") // FIXME FIXME?
}

package gluster

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	"github.com/containers/image/directory/explicitfilepath"
	"github.com/containers/image/docker/reference"
	"github.com/containers/image/image"
	"github.com/containers/image/transports"
	"github.com/containers/image/types"
	"github.com/opencontainers/go-digest"
)

func init() {
	transports.Register(Transport)
}

// Transport is an ImageTransport for directory paths.
var Transport = glusterTransport{}

type glusterTransport struct{}

func (t glusterTransport) Name() string {
	return "gluster"
}

// ParseReference converts a string, which should not start with the ImageTransport.Name prefix, into an ImageReference.
func (t glusterTransport) ParseReference(reference string) (types.ImageReference, error) {
	return NewReference(reference)
}

// ValidatePolicyConfigurationScope checks that scope is a valid name for a signature.PolicyTransportScopes keys
// (i.e. a valid PolicyConfigurationIdentity() or PolicyConfigurationNamespaces() return value).
// It is acceptable to allow an invalid value which will never be matched, it can "only" cause user confusion.
// scope passed to this function will not be "", that value is always allowed.
func (t glusterTransport) ValidatePolicyConfigurationScope(scope string) error {
	if !strings.HasPrefix(scope, "/") {
		return errors.Errorf("Invalid scope %s: Must be an absolute path", scope)
	}
	// Refuse also "/", otherwise "/" and "" would have the same semantics,
	// and "" could be unexpectedly shadowed by the "/" entry.
	if scope == "/" {
		return errors.New(`Invalid scope "/": Use the generic default scope ""`)
	}
	cleaned := filepath.Clean(scope)
	if cleaned != scope {
		return errors.Errorf(`Invalid scope %s: Uses non-canonical format, perhaps try %s`, scope, cleaned)
	}
	return nil
}

// glusterReference is an ImageReference for directory paths.
type glusterReference struct {
	// Note that the interpretation of paths below depends on the underlying filesystem state, which may change under us at any time!
	// Either of the paths may point to a different, or no, inode over time.  resolvedPath may contain symbolic links, and so on.

	// Generally we follow the intent of the user, and use the "path" member for filesystem operations (e.g. the user can use a relative path to avoid
	// being exposed to symlinks and renames in the parent directories to the working directory).
	// (But in general, we make no attempt to be completely safe against concurrent hostile filesystem modifications.)
	path         string // As specified by the user. May be relative, contain symlinks, etc.
	resolvedPath string // Absolute path with no symlinks, at least at the time of its creation. Primarily used for policy namespaces.
}

// There is no directory.ParseReference because it is rather pointless.
// Callers who need a transport-independent interface will go through
// glusterTransport.ParseReference; callers who intentionally deal with directories
// can use directory.NewReference.

// NewReference returns a directory reference for a specified path.
//
// We do not expose an API supplying the resolvedPath; we could, but recomputing it
// is generally cheap enough that we prefer being confident about the properties of resolvedPath.
func NewReference(path string) (types.ImageReference, error) {
	resolved, err := explicitfilepath.ResolvePathToFullyExplicit(path)
	if err != nil {
		return nil, err
	}
	return glusterReference{path: path, resolvedPath: resolved}, nil
}

func (ref glusterReference) Transport() types.ImageTransport {
	return Transport
}

// StringWithinTransport returns a string representation of the reference, which MUST be such that
// reference.Transport().ParseReference(reference.StringWithinTransport()) returns an equivalent reference.
// NOTE: The returned string is not promised to be equal to the original input to ParseReference;
// e.g. default attribute values omitted by the user may be filled in in the return value, or vice versa.
// WARNING: Do not use the return value in the UI to describe an image, it does not contain the Transport().Name() prefix.
func (ref glusterReference) StringWithinTransport() string {
	return ref.path
}

// DockerReference returns a Docker reference associated with this reference
// (fully explicit, i.e. !reference.IsNameOnly, but reflecting user intent,
// not e.g. after redirect or alias processing), or nil if unknown/not applicable.
func (ref glusterReference) DockerReference() reference.Named {
	return nil
}

// PolicyConfigurationIdentity returns a string representation of the reference, suitable for policy lookup.
// This MUST reflect user intent, not e.g. after processing of third-party redirects or aliases;
// The value SHOULD be fully explicit about its semantics, with no hidden defaults, AND canonical
// (i.e. various references with exactly the same semantics should return the same configuration identity)
// It is fine for the return value to be equal to StringWithinTransport(), and it is desirable but
// not required/guaranteed that it will be a valid input to Transport().ParseReference().
// Returns "" if configuration identities for these references are not supported.
func (ref glusterReference) PolicyConfigurationIdentity() string {
	return ref.resolvedPath
}

// PolicyConfigurationNamespaces returns a list of other policy configuration namespaces to search
// for if explicit configuration for PolicyConfigurationIdentity() is not set.  The list will be processed
// in order, terminating on first match, and an implicit "" is always checked at the end.
// It is STRONGLY recommended for the first element, if any, to be a prefix of PolicyConfigurationIdentity(),
// and each following element to be a prefix of the element preceding it.
func (ref glusterReference) PolicyConfigurationNamespaces() []string {
	res := []string{}
	path := ref.resolvedPath
	for {
		lastSlash := strings.LastIndex(path, "/")
		if lastSlash == -1 || lastSlash == 0 {
			break
		}
		path = path[:lastSlash]
		res = append(res, path)
	}
	// Note that we do not include "/"; it is redundant with the default "" global default,
	// and rejected by glusterTransport.ValidatePolicyConfigurationScope above.
	return res
}

// NewImage returns a types.Image for this reference, possibly specialized for this ImageTransport.
// The caller must call .Close() on the returned Image.
// NOTE: If any kind of signature verification should happen, build an UnparsedImage from the value returned by NewImageSource,
// verify that UnparsedImage, and convert it into a real Image via image.FromUnparsedImage.
func (ref glusterReference) NewImage(ctx *types.SystemContext) (types.Image, error) {
	src := newImageSource(ref)
	return image.FromSource(src)
}

// NewImageSource returns a types.ImageSource for this reference,
// asking the backend to use a manifest from requestedManifestMIMETypes if possible.
// nil requestedManifestMIMETypes means manifest.DefaultRequestedManifestMIMETypes.
// The caller must call .Close() on the returned ImageSource.
func (ref glusterReference) NewImageSource(ctx *types.SystemContext, requestedManifestMIMETypes []string) (types.ImageSource, error) {
	return newImageSource(ref), nil
}

// NewImageDestination returns a types.ImageDestination for this reference.
// The caller must call .Close() on the returned ImageDestination.
func (ref glusterReference) NewImageDestination(ctx *types.SystemContext) (types.ImageDestination, error) {
	return newImageDestination(ref), nil
}

// DeleteImage deletes the named image from the registry, if supported.
func (ref glusterReference) DeleteImage(ctx *types.SystemContext) error {
	return errors.Errorf("Deleting images not implemented for dir: images")
}

// manifestPath returns a path for the manifest within a directory using our conventions.
func (ref glusterReference) manifestPath() string {
	return filepath.Join(ref.path, "manifest.json")
}

// layerPath returns a path for a layer tarball within a directory using our conventions.
func (ref glusterReference) layerPath(digest digest.Digest) string {
	// FIXME: Should we keep the digest identification?
	return filepath.Join(ref.path, digest.Hex()+".tar")
}

// signaturePath returns a path for a signature within a directory using our conventions.
func (ref glusterReference) signaturePath(index int) string {
	return filepath.Join(ref.path, fmt.Sprintf("signature-%d", index+1))
}

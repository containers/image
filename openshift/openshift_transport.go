package openshift

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"

	"github.com/Sirupsen/logrus"
	"github.com/containers/image/types"
	"github.com/docker/docker/reference"
)

// Transport is an ImageTransport for directory paths.
var Transport = openshiftTransport{}

type openshiftTransport struct{}

func (t openshiftTransport) Name() string {
	return "atomic"
}

// ParseReference converts a string, which should not start with the ImageTransport.Name prefix, into an ImageReference.
func (t openshiftTransport) ParseReference(reference string) (types.ImageReference, error) {
	return ParseReference(reference)
}

// openshiftReference is an ImageReference for OpenShift images.
type openshiftReference struct {
	baseURL         *url.URL
	namespace       string
	stream          string
	tag             string
	dockerReference reference.Named // Computed from the above in advance, so that later references can not fail.
}

// FIXME: Is imageName like this a good way to refer to OpenShift images?
var imageNameRegexp = regexp.MustCompile("^([^:/]*)/([^:/]*):([^:/]*)$")

// ParseReference converts a string, which should not start with the ImageTransport.Name prefix, into an OpenShift ImageReference.
func ParseReference(reference string) (types.ImageReference, error) {
	// Overall, this is modelled on openshift/origin/pkg/cmd/util/clientcmd.New().ClientConfig() and openshift/origin/pkg/client.
	cmdConfig := defaultClientConfig()
	logrus.Debugf("cmdConfig: %#v", cmdConfig)
	restConfig, err := cmdConfig.ClientConfig()
	if err != nil {
		return nil, err
	}
	// REMOVED: SetOpenShiftDefaults (values are not overridable in config files, so hard-coded these defaults.)
	logrus.Debugf("restConfig: %#v", restConfig)
	baseURL, _, err := restClientFor(restConfig)
	if err != nil {
		return nil, err
	}
	logrus.Debugf("URL: %#v", *baseURL)

	m := imageNameRegexp.FindStringSubmatch(reference)
	if m == nil || len(m) != 4 {
		return nil, fmt.Errorf("Invalid image reference %s, %#v", reference, m)
	}

	return NewReference(baseURL, m[1], m[2], m[3])
}

// NewReference returns an OpenShift reference for a base URL, namespace, stream and tag.
func NewReference(baseURL *url.URL, namespace, stream, tag string) (types.ImageReference, error) {
	// Precompute also dockerReference so that later references can not fail.
	//
	// This discards ref.baseURL.Path, which is unexpected for a “base URL”;
	// but openshiftClient.doRequest actually completely overrides url.Path
	// (and defaultServerURL rejects non-trivial Path values), so it is OK for
	// us to ignore it as well.
	//
	// FIXME: This is, strictly speaking, a namespace conflict with images placed in a Docker registry running on the same host.
	// Do we need to do something else, perhaps disambiguate (port number?) or namespace Docker and OpenShift separately?
	dockerRef, err := reference.WithName(fmt.Sprintf("%s/%s/%s", baseURL.Host, namespace, stream))
	if err != nil {
		return nil, err
	}
	dockerRef, err = reference.WithTag(dockerRef, tag)
	if err != nil {
		return nil, err
	}

	return openshiftReference{
		baseURL:         baseURL,
		namespace:       namespace,
		stream:          stream,
		tag:             tag,
		dockerReference: dockerRef,
	}, nil
}

func (ref openshiftReference) Transport() types.ImageTransport {
	return Transport
}

// StringWithinTransport returns a string representation of the reference, which MUST be such that
// reference.Transport().ParseReference(reference.StringWithinTransport()) returns an equivalent reference.
// NOTE: The returned string is not promised to be equal to the original input to ParseReference;
// e.g. default attribute values omitted by the user may be filled in in the return value, or vice versa.
// WARNING: Do not use the return value in the UI to describe an image, it does not contain the Transport().Name() prefix.
func (ref openshiftReference) StringWithinTransport() string {
	return fmt.Sprintf("%s/%s:%s", ref.namespace, ref.stream, ref.tag)
}

// DockerReference returns a Docker reference associated with this reference
// (fully explicit, i.e. !reference.IsNameOnly, but reflecting user intent,
// not e.g. after redirect or alias processing), or nil if unknown/not applicable.
func (ref openshiftReference) DockerReference() reference.Named {
	return ref.dockerReference
}

// NewImage returns a types.Image for this reference.
func (ref openshiftReference) NewImage(certPath string, tlsVerify bool) (types.Image, error) {
	return nil, errors.New("Full Image support not implemented for atomic: image names")
}

// NewImageSource returns a types.ImageSource for this reference.
func (ref openshiftReference) NewImageSource(certPath string, tlsVerify bool) (types.ImageSource, error) {
	return newImageSource(ref, certPath, tlsVerify)
}

// NewImageDestination returns a types.ImageDestination for this reference.
func (ref openshiftReference) NewImageDestination(certPath string, tlsVerify bool) (types.ImageDestination, error) {
	return newImageDestination(ref, certPath, tlsVerify)
}

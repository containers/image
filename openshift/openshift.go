package openshift

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/containers/image/docker"
	"github.com/containers/image/manifest"
	"github.com/containers/image/types"
	"github.com/containers/image/version"
	"github.com/docker/docker/reference"
)

// openshiftClient is configuration for dealing with a single image stream, for reading or writing.
type openshiftClient struct {
	// Values from Kubernetes configuration
	baseURL     *url.URL
	httpClient  *http.Client
	bearerToken string // "" if not used
	username    string // "" if not used
	password    string // if username != ""
	// Values specific to this image
	namespace                string
	stream                   string
	tag                      string
	canonicalDockerReference reference.Named // Computed from the above in advance, so that later references can not fail.
}

// FIXME: Is imageName like this a good way to refer to OpenShift images?
var imageNameRegexp = regexp.MustCompile("^([^:/]*)/([^:/]*):([^:/]*)$")

// newOpenshiftClient creates a new openshiftClient for the specified image.
func newOpenshiftClient(imageName string) (*openshiftClient, error) {
	// Overall, this is modelled on openshift/origin/pkg/cmd/util/clientcmd.New().ClientConfig() and openshift/origin/pkg/client.
	cmdConfig := defaultClientConfig()
	logrus.Debugf("cmdConfig: %#v", cmdConfig)
	restConfig, err := cmdConfig.ClientConfig()
	if err != nil {
		return nil, err
	}
	// REMOVED: SetOpenShiftDefaults (values are not overridable in config files, so hard-coded these defaults.)
	logrus.Debugf("restConfig: %#v", restConfig)
	baseURL, httpClient, err := restClientFor(restConfig)
	if err != nil {
		return nil, err
	}
	logrus.Debugf("URL: %#v", *baseURL)

	m := imageNameRegexp.FindStringSubmatch(imageName)
	if m == nil || len(m) != 4 {
		return nil, fmt.Errorf("Invalid image reference %s, %#v", imageName, m)
	}

	c := &openshiftClient{
		baseURL:     baseURL,
		httpClient:  httpClient,
		bearerToken: restConfig.BearerToken,
		username:    restConfig.Username,
		password:    restConfig.Password,

		namespace: m[1],
		stream:    m[2],
		tag:       m[3],
	}

	// Precompute also c.canonicalDockerReference so that later references can not fail.
	// FIXME: This is, strictly speaking, a namespace conflict with images placed in a Docker registry running on the same host.
	// Do we need to do something else, perhaps disambiguate (port number?) or namespace Docker and OpenShift separately?
	dockerRef, err := reference.WithName(fmt.Sprintf("%s/%s/%s", c.baseURL.Host, c.namespace, c.stream))
	if err != nil {
		return nil, err
	}
	c.canonicalDockerReference, err = reference.WithTag(dockerRef, c.tag)
	if err != nil {
		return nil, err
	}

	return c, nil
}

// doRequest performs a correctly authenticated request to a specified path, and returns response body or an error object.
func (c *openshiftClient) doRequest(method, path string, requestBody []byte) ([]byte, error) {
	url := *c.baseURL
	url.Path = path
	var requestBodyReader io.Reader
	if requestBody != nil {
		logrus.Debugf("Will send body: %s", requestBody)
		requestBodyReader = bytes.NewReader(requestBody)
	}
	req, err := http.NewRequest(method, url.String(), requestBodyReader)
	if err != nil {
		return nil, err
	}

	if len(c.bearerToken) != 0 {
		req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	} else if len(c.username) != 0 {
		req.SetBasicAuth(c.username, c.password)
	}
	req.Header.Set("Accept", "application/json, */*")
	req.Header.Set("User-Agent", fmt.Sprintf("skopeo/%s", version.Version))
	if requestBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	logrus.Debugf("%s %s", method, url)
	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	logrus.Debugf("Got body: %s", body)
	// FIXME: Just throwing this useful information away only to try to guess later...
	logrus.Debugf("Got content-type: %s", res.Header.Get("Content-Type"))

	var status status
	statusValid := false
	if err := json.Unmarshal(body, &status); err == nil && len(status.Status) > 0 {
		statusValid = true
	}

	switch {
	case res.StatusCode == http.StatusSwitchingProtocols: // FIXME?! No idea why this weird case exists in k8s.io/kubernetes/pkg/client/restclient.
		if statusValid && status.Status != "Success" {
			return nil, errors.New(status.Message)
		}
	case res.StatusCode >= http.StatusOK && res.StatusCode <= http.StatusPartialContent:
		// OK.
	default:
		if statusValid {
			return nil, errors.New(status.Message)
		}
		return nil, fmt.Errorf("HTTP error: status code: %d, body: %s", res.StatusCode, string(body))
	}

	return body, nil
}

// convertDockerImageReference takes an image API DockerImageReference value and returns a reference we can actually use;
// currently OpenShift stores the cluster-internal service IPs here, which are unusable from the outside.
func (c *openshiftClient) convertDockerImageReference(ref string) (string, error) {
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("Invalid format of docker reference %s: missing '/'", ref)
	}
	// Sanity check that the reference is at least plausibly similar, i.e. uses the hard-coded port we expect.
	if !strings.HasSuffix(parts[0], ":5000") {
		return "", fmt.Errorf("Invalid format of docker reference %s: expecting port 5000", ref)
	}
	return c.dockerRegistryHostPart() + "/" + parts[1], nil
}

// dockerRegistryHostPart returns the host:port of the embedded Docker Registry API endpoint
// FIXME: There seems to be no way to discover the correct:host port using the API, so hard-code our knowledge
// about how the OpenShift Atomic Registry is configured, per examples/atomic-registry/run.sh:
// -p OPENSHIFT_OAUTH_PROVIDER_URL=https://${INSTALL_HOST}:8443,COCKPIT_KUBE_URL=https://${INSTALL_HOST},REGISTRY_HOST=${INSTALL_HOST}:5000
func (c *openshiftClient) dockerRegistryHostPart() string {
	return strings.SplitN(c.baseURL.Host, ":", 2)[0] + ":5000"
}

type openshiftImageSource struct {
	client *openshiftClient
	// Values specific to this image
	certPath  string // Only for parseDockerImageSource
	tlsVerify bool   // Only for parseDockerImageSource
	username  string // Only for parseDockerImageSource
	password  string // Only for parseDockerImageSource
	// State
	docker               types.ImageSource // The Docker Registry endpoint, or nil if not resolved yet
	imageStreamImageName string            // Resolved image identifier, or "" if not known yet
}

// NewImageSource creates a new ImageSource for the specified image and connection specification.
func NewImageSource(imageName, certPath string, tlsVerify bool, username, password string) (types.ImageSource, error) {
	client, err := newOpenshiftClient(imageName)
	if err != nil {
		return nil, err
	}

	return &openshiftImageSource{
		client:    client,
		certPath:  certPath,
		tlsVerify: tlsVerify,
		username:  username,
		password:  password,
	}, nil
}

// IntendedDockerReference returns the Docker reference for this image, _as specified by the user_
// (not as the image itself, or its underlying storage, claims).  Should be fully expanded, i.e. !reference.IsNameOnly.
// This can be used e.g. to determine which public keys are trusted for this image.
// May be nil if unknown.
func (s *openshiftImageSource) IntendedDockerReference() reference.Named {
	return s.client.canonicalDockerReference
}

func (s *openshiftImageSource) GetManifest(mimetypes []string) ([]byte, string, error) {
	if err := s.ensureImageIsResolved(); err != nil {
		return nil, "", err
	}
	return s.docker.GetManifest(mimetypes)
}

func (s *openshiftImageSource) GetBlob(digest string) (io.ReadCloser, int64, error) {
	if err := s.ensureImageIsResolved(); err != nil {
		return nil, 0, err
	}
	return s.docker.GetBlob(digest)
}

func (s *openshiftImageSource) GetSignatures() ([][]byte, error) {
	return nil, nil
}

// ensureImageIsResolved sets up s.docker and s.imageStreamImageName
func (s *openshiftImageSource) ensureImageIsResolved() error {
	if s.docker != nil {
		return nil
	}

	// FIXME: validate components per validation.IsValidPathSegmentName?
	path := fmt.Sprintf("/oapi/v1/namespaces/%s/imagestreams/%s", s.client.namespace, s.client.stream)
	body, err := s.client.doRequest("GET", path, nil)
	if err != nil {
		return err
	}
	// Note: This does absolutely no kind/version checking or conversions.
	var is imageStream
	if err := json.Unmarshal(body, &is); err != nil {
		return err
	}
	var te *tagEvent
	for _, tag := range is.Status.Tags {
		if tag.Tag != s.client.tag {
			continue
		}
		if len(tag.Items) > 0 {
			te = &tag.Items[0]
			break
		}
	}
	if te == nil {
		return fmt.Errorf("No matching tag found")
	}
	logrus.Debugf("tag event %#v", te)
	dockerRef, err := s.client.convertDockerImageReference(te.DockerImageReference)
	if err != nil {
		return err
	}
	logrus.Debugf("Resolved reference %#v", dockerRef)
	d, err := docker.NewImageSource(dockerRef, s.certPath, s.tlsVerify, s.username, s.password)
	if err != nil {
		return err
	}
	s.docker = d
	s.imageStreamImageName = te.Image
	return nil
}

type openshiftImageDestination struct {
	client *openshiftClient
	docker types.ImageDestination // The Docker Registry endpoint
}

// NewImageDestination creates a new ImageDestination for the specified image and connection specification.
func NewImageDestination(imageName, certPath string, tlsVerify bool, username, password string) (types.ImageDestination, error) {
	client, err := newOpenshiftClient(imageName)
	if err != nil {
		return nil, err
	}

	// FIXME: Should this always use a digest, not a tag? Uploading to Docker by tag requires the tag _inside_ the manifest to match,
	// i.e. a single signed image cannot be available under multiple tags.  But with types.ImageDestination, we don't know
	// the manifest digest at this point.
	dockerRef := fmt.Sprintf("%s/%s/%s:%s", client.dockerRegistryHostPart(), client.namespace, client.stream, client.tag)
	docker, err := docker.NewImageDestination(dockerRef, certPath, tlsVerify, username, password)
	if err != nil {
		return nil, err
	}

	return &openshiftImageDestination{
		client: client,
		docker: docker,
	}, nil
}

func (d *openshiftImageDestination) SupportedManifestMIMETypes() []string {
	return []string{
		manifest.DockerV2Schema1SignedMIMEType,
		manifest.DockerV2Schema1MIMEType,
	}
}

func (d *openshiftImageDestination) CanonicalDockerReference() reference.Named {
	return d.client.canonicalDockerReference
}

func (d *openshiftImageDestination) PutManifest(m []byte) error {
	// Note: This does absolutely no kind/version checking or conversions.
	manifestDigest, err := manifest.Digest(m)
	if err != nil {
		return err
	}
	// FIXME: We can't do what respositorymiddleware.go does because we don't know the internal address. Does any of this matter?
	dockerImageReference := fmt.Sprintf("%s/%s/%s@%s", d.client.dockerRegistryHostPart(), d.client.namespace, d.client.stream, manifestDigest)
	ism := imageStreamMapping{
		typeMeta: typeMeta{
			Kind:       "ImageStreamMapping",
			APIVersion: "v1",
		},
		objectMeta: objectMeta{
			Namespace: d.client.namespace,
			Name:      d.client.stream,
		},
		Image: image{
			objectMeta: objectMeta{
				Name: manifestDigest,
			},
			DockerImageReference: dockerImageReference,
			DockerImageManifest:  string(m),
		},
		Tag: d.client.tag,
	}
	body, err := json.Marshal(ism)
	if err != nil {
		return err
	}

	// FIXME: validate components per validation.IsValidPathSegmentName?
	path := fmt.Sprintf("/oapi/v1/namespaces/%s/imagestreammappings", d.client.namespace)
	body, err = d.client.doRequest("POST", path, body)
	if err != nil {
		return err
	}

	return d.docker.PutManifest(m)
}

func (d *openshiftImageDestination) PutBlob(digest string, stream io.Reader) error {
	return d.docker.PutBlob(digest, stream)
}

func (d *openshiftImageDestination) PutSignatures(signatures [][]byte) error {
	if len(signatures) != 0 {
		return fmt.Errorf("Pushing signatures to an Atomic Registry is not supported")
	}
	return nil
}

// These structs are subsets of github.com/openshift/origin/pkg/image/api/v1 and its dependencies.
type imageStream struct {
	Status imageStreamStatus `json:"status,omitempty"`
}
type imageStreamStatus struct {
	DockerImageRepository string              `json:"dockerImageRepository"`
	Tags                  []namedTagEventList `json:"tags,omitempty"`
}
type namedTagEventList struct {
	Tag   string     `json:"tag"`
	Items []tagEvent `json:"items"`
}
type tagEvent struct {
	DockerImageReference string `json:"dockerImageReference"`
	Image                string `json:"image"`
}
type imageStreamImage struct {
	Image image `json:"image"`
}
type image struct {
	objectMeta           `json:"metadata,omitempty"`
	DockerImageReference string `json:"dockerImageReference,omitempty"`
	//	DockerImageMetadata        runtime.RawExtension `json:"dockerImageMetadata,omitempty"`
	DockerImageMetadataVersion string `json:"dockerImageMetadataVersion,omitempty"`
	DockerImageManifest        string `json:"dockerImageManifest,omitempty"`
	//	DockerImageLayers          []ImageLayer         `json:"dockerImageLayers"`
}
type imageStreamMapping struct {
	typeMeta   `json:",inline"`
	objectMeta `json:"metadata,omitempty"`
	Image      image  `json:"image"`
	Tag        string `json:"tag"`
}
type typeMeta struct {
	Kind       string `json:"kind,omitempty"`
	APIVersion string `json:"apiVersion,omitempty"`
}
type objectMeta struct {
	Name                       string            `json:"name,omitempty"`
	GenerateName               string            `json:"generateName,omitempty"`
	Namespace                  string            `json:"namespace,omitempty"`
	SelfLink                   string            `json:"selfLink,omitempty"`
	ResourceVersion            string            `json:"resourceVersion,omitempty"`
	Generation                 int64             `json:"generation,omitempty"`
	DeletionGracePeriodSeconds *int64            `json:"deletionGracePeriodSeconds,omitempty"`
	Labels                     map[string]string `json:"labels,omitempty"`
	Annotations                map[string]string `json:"annotations,omitempty"`
}

// A subset of k8s.io/kubernetes/pkg/api/unversioned/Status
type status struct {
	Status  string `json:"status,omitempty"`
	Message string `json:"message,omitempty"`
	// Reason StatusReason `json:"reason,omitempty"`
	// Details *StatusDetails `json:"details,omitempty"`
	Code int32 `json:"code,omitempty"`
}

func (s *openshiftImageSource) Delete() error {
	return fmt.Errorf("openshift#openshiftImageSource.Delete() not implmented")
}

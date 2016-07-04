package docker

import (
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"strconv"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/reference"
	"github.com/containers/image/manifest"
	"github.com/containers/image/types"
)

type errFetchManifest struct {
	statusCode int
	body       []byte
}

func (e errFetchManifest) Error() string {
	return fmt.Sprintf("error fetching manifest: status code: %d, body: %s", e.statusCode, string(e.body))
}

type dockerImageSource struct {
	ref reference.Named
	tag string
	c   Client
}

// newDockerImageSource is the same as NewImageSource, only it returns the more specific *dockerImageSource type.
func newDockerImageSource(img string, dc Client) (*dockerImageSource, error) {
	ref, tag, err := parseDockerImageName(img)
	if err != nil {
		return nil, err
	}
	return &dockerImageSource{
		ref: ref,
		tag: tag,
		c:   dc,
	}, nil
}

// NewImageSource creates a new ImageSource for the specified image and connection specification.
func NewImageSource(img string, dc Client) (types.ImageSource, error) {
	return newDockerImageSource(img, dc)
}

// IntendedDockerReference returns the full, unambiguous, Docker reference for this image, _as specified by the user_
// (not as the image itself, or its underlying storage, claims).  This can be used e.g. to determine which public keys are trusted for this image.
// May be "" if unknown.
func (s *dockerImageSource) IntendedDockerReference() string {
	return fmt.Sprintf("%s:%s", s.ref.Name(), s.tag)
}

// simplifyContentType drops parameters from a HTTP media type (see https://tools.ietf.org/html/rfc7231#section-3.1.1.1)
// Alternatively, an empty string is returned unchanged, and invalid values are "simplified" to an empty string.
func simplifyContentType(contentType string) string {
	if contentType == "" {
		return contentType
	}
	mimeType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return ""
	}
	return mimeType
}

func (s *dockerImageSource) GetManifest(mimetypes []string) ([]byte, string, error) {
	url := fmt.Sprintf(manifestURL, s.ref.RemoteName(), s.tag)
	// TODO(runcom) set manifest version header! schema1 for now - then schema2 etc etc and v1
	// TODO(runcom) NO, switch on the resulter manifest like Docker is doing
	headers := make(map[string][]string)
	headers["Accept"] = mimetypes
	res, err := s.c.MakeRequest("GET", url, headers, nil, false)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	manblob, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, "", err
	}
	if res.StatusCode != http.StatusOK {
		return nil, "", errFetchManifest{res.StatusCode, manblob}
	}
	// We might validate manblob against the Docker-Content-Digest header here to protect against transport errors.
	return manblob, simplifyContentType(res.Header.Get("Content-Type")), nil
}

func (s *dockerImageSource) GetBlob(digest string) (io.ReadCloser, int64, error) {
	url := fmt.Sprintf(blobsURL, s.ref.RemoteName(), digest)
	logrus.Debugf("Downloading %s", url)
	res, err := s.c.MakeRequest("GET", url, nil, nil, false)
	if err != nil {
		return nil, 0, err
	}
	if res.StatusCode != http.StatusOK {
		// print url also
		return nil, 0, fmt.Errorf("Invalid status code returned when fetching blob %d", res.StatusCode)
	}
	size, err := strconv.ParseInt(res.Header.Get("Content-Length"), 10, 64)
	if err != nil {
		size = 0
	}
	return res.Body, size, nil
}

func (s *dockerImageSource) GetSignatures() ([][]byte, error) {
	return [][]byte{}, nil
}

func (s *dockerImageSource) Delete() error {
	var body []byte

	// When retrieving the digest from a registry >= 2.3 use the following header:
	//   "Accept": "application/vnd.docker.distribution.manifest.v2+json"
	headers := make(map[string][]string)
	headers["Accept"] = []string{manifest.DockerV2Schema2MIMEType}

	getURL := fmt.Sprintf(manifestURL, s.ref.RemoteName(), s.tag)
	get, err := s.c.MakeRequest("GET", getURL, headers, nil, false)
	if err != nil {
		return err
	}
	defer get.Body.Close()
	body, err = ioutil.ReadAll(get.Body)
	if err != nil {
		return err
	}
	switch get.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return fmt.Errorf("Unable to delete %v. Image may not exist or is not stored with a v2 Schema in a v2 registry.", s.ref)
	default:
		return fmt.Errorf("Failed to delete %v: %v (%v)", s.ref, body, get.Status)
	}

	digest := get.Header.Get("Docker-Content-Digest")
	deleteURL := fmt.Sprintf(manifestURL, s.ref.RemoteName(), digest)

	// When retrieving the digest from a registry >= 2.3 use the following header:
	//   "Accept": "application/vnd.docker.distribution.manifest.v2+json"
	delete, err := s.c.MakeRequest("DELETE", deleteURL, headers, nil, false)
	if err != nil {
		return err
	}
	defer delete.Body.Close()

	body, err = ioutil.ReadAll(delete.Body)
	if err != nil {
		return err
	}
	if delete.StatusCode != http.StatusAccepted {
		return fmt.Errorf("Failed to delete %v: %v (%v)", deleteURL, body, delete.Status)
	}

	return nil
}

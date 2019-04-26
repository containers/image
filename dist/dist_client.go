package dist

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"github.com/containers/image/v5/pkg/docker/config"
	"github.com/containers/image/v5/pkg/tlsclientconfig"
	"github.com/containers/image/v5/types"
	"github.com/opencontainers/go-digest"
	ispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/pkg/errors"
)

type OciRepo struct {
	url       url.URL
	ref       *distReference
	authCreds string
	client    *http.Client
}

//nolint (funlen)
func NewOciRepo(ref *distReference, sys *types.SystemContext) (r OciRepo, err error) {
	server := "127.0.0.1"
	port := "8080"
	hostName := ""

	if ref.server != "" {
		server = ref.server
		hostName = server
	}

	if ref.port != -1 {
		port = fmt.Sprintf("%d", ref.port)
		hostName += ":" + port
	}

	insecureSkipVerify := false
	if sys != nil {
		insecureSkipVerify = (sys.DockerInsecureSkipTLSVerify == types.OptionalBoolTrue)
	}

	tlsClientConfig := &tls.Config{
		MinVersion:               tls.VersionTLS10,
		PreferServerCipherSuites: true,
		InsecureSkipVerify:       insecureSkipVerify,
	}

	certDir, err := ociCertDir(sys, hostName)
	if err != nil {
		return r, err
	}
	if err := tlsclientconfig.SetupCertificates(certDir, tlsClientConfig); err != nil {
		return r, err
	}

	transport := &http.Transport{TLSClientConfig: tlsClientConfig}
	client := &http.Client{Transport: transport}
	creds := ""
	if sys != nil {
		if sys.DockerAuthConfig != nil {
			a := sys.DockerAuthConfig
			creds = base64.StdEncoding.EncodeToString([]byte(a.Username + ":" + a.Password))
		} else {
			registry := fmt.Sprintf("%s:%s", server, port)
			if username, password, err := config.GetAuthentication(sys, registry); err == nil {
				creds = base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
			}
		}
	}

	r = OciRepo{ref: ref, authCreds: creds, client: client}

	ping := func(scheme string) error {
		u := url.URL{Scheme: scheme, Host: fmt.Sprintf("%s:%s", server, port), Path: fmt.Sprintf("/v2/")}
		resp, err := client.Get(u.String())
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusUnauthorized {
			return errors.Errorf("Error pinging registry %s:%s, response code %d (%s)",
				server, port, resp.StatusCode, http.StatusText(resp.StatusCode))
		}
		return nil
	}

	scheme := "https"
	err = ping(scheme)
	if err != nil && insecureSkipVerify {
		scheme = "http"
		err = ping(scheme)
	}
	if err != nil {
		return r, errors.Wrap(err, "unable to ping registry")
	}

	r.url = url.URL{Scheme: scheme, Host: fmt.Sprintf("%s:%s", server, port)}
	return r, nil
}

func (o *OciRepo) GetManifest() ([]byte, *ispec.Manifest, error) {
	name := o.ref.name
	tag := o.ref.tag
	m := &ispec.Manifest{}

	var body []byte

	manifestURI, err := url.Parse(fmt.Sprintf("/v2/%s/manifests/%s", name, tag))
	if err != nil {
		return body, m, errors.Wrapf(err, "couldn't parse manifest url for repo %s and image %s", name, tag)
	}

	manifestURI = o.url.ResolveReference(manifestURI)
	req, err := http.NewRequest(http.MethodGet, manifestURI.String(), nil)

	if err != nil {
		return body, m, errors.Wrapf(err, "Couldn't create DELETE request for %v", manifestURI)
	}

	if o.authCreds != "" {
		req.Header.Add("Authorization", "Basic "+o.authCreds)
	}

	req.Header.Add("Accept", "application/vnd.oci.image.manifest.v1+json")
	resp, err := o.client.Do(req)

	if err != nil {
		return body, m, errors.Wrapf(err, "Error getting manifest %s %s from %v", name, tag, o.url)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return body, m, fmt.Errorf("bad return code %d getting manifest", resp.StatusCode)
	}

	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return body, m, errors.Wrapf(err, "Error reading response body for %s", tag)
	}

	err = json.Unmarshal(body, m)

	if err != nil {
		return body, m, errors.Wrap(err, "Failed decoding response")
	}

	return body, m, nil
}

func (o *OciRepo) RemoveManifest() error {
	name := o.ref.name
	tag := o.ref.tag
	manifestURI, err := url.Parse(fmt.Sprintf("/v2/%s/manifests/%s", name, tag))

	if err != nil {
		return errors.Wrapf(err, "couldn't parse manifest url for repo %s and image %s", name, tag)
	}

	manifestURI = o.url.ResolveReference(manifestURI)
	req, err := http.NewRequest(http.MethodDelete, manifestURI.String(), nil)

	if err != nil {
		return errors.Wrapf(err, "Couldn't create DELETE request for %v", manifestURI)
	}

	if o.authCreds != "" {
		req.Header.Add("Authorization", "Basic "+o.authCreds)
	}

	resp, err := o.client.Do(req)
	if err != nil {
		return errors.Wrapf(err, "Error deleting manifest")
	}

	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("server returned unexpected code %d", resp.StatusCode)
	}

	defer resp.Body.Close()

	return nil
}

func (o *OciRepo) PutManifest(body []byte) error {
	name := o.ref.name
	tag := o.ref.tag
	manifestURI, err := url.Parse(fmt.Sprintf("/v2/%s/manifests/%s", name, tag))

	if err != nil {
		return errors.Wrapf(err, "couldn't parse manifest url for repo %s and image %s", name, tag)
	}

	manifestURI = o.url.ResolveReference(manifestURI)
	req, err := http.NewRequest(http.MethodPut, manifestURI.String(), bytes.NewReader(body))

	if err != nil {
		return errors.Wrapf(err, "Couldn't create PUT request for %v", manifestURI)
	}

	if o.authCreds != "" {
		req.Header.Add("Authorization", "Basic "+o.authCreds)
	}

	req.Header.Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
	resp, err := o.client.Do(req)

	if err != nil {
		return errors.Wrapf(err, "Error posting manifest")
	}

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("server returned unexpected code %d", resp.StatusCode)
	}

	defer resp.Body.Close()

	return nil
}

//HEAD /v2/<name>/blobs/<digest>  -> 200 (has layer)
func (o *OciRepo) HasLayer(ldigest string) bool {
	name := o.ref.name
	blobURI, err := url.Parse(fmt.Sprintf("/v2/%s/blobs/%s", name, ldigest))

	if err != nil {
		return false
	}

	blobURI = o.url.ResolveReference(blobURI)
	req, err := http.NewRequest(http.MethodHead, blobURI.String(), nil)

	if err != nil {
		return false
	}

	if o.authCreds != "" {
		req.Header.Add("Authorization", "Basic "+o.authCreds)
	}

	resp, err := o.client.Do(req)

	if err != nil {
		return false
	}

	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

func (o *OciRepo) GetLayer(ldigest string) (io.ReadCloser, int64, error) {
	name := o.ref.name
	blobURI, err := url.Parse(fmt.Sprintf("/v2/%s/blobs/%s", name, ldigest))

	if err != nil {
		return nil, -1, errors.Wrapf(err, "couldn't parse URL for repo %s blob %s", name, ldigest)
	}

	blobURI = o.url.ResolveReference(blobURI)
	req, err := http.NewRequest("GET", blobURI.String(), nil)

	if err != nil {
		return nil, -1, errors.Wrapf(err, "Couldn't create GET request for %v", blobURI)
	}

	if o.authCreds != "" {
		req.Header.Add("Authorization", "Basic "+o.authCreds)
	}

	resp, err := o.client.Do(req)

	if err != nil {
		return nil, -1, errors.Wrapf(err, "Error getting layer %s", ldigest)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, -1, fmt.Errorf("bad return code %d getting layer", resp.StatusCode)
	}

	return resp.Body, -1, err
}

// StartLayer starts a blob upload session
func (o *OciRepo) StartLayer() (*url.URL, error) {
	name := o.ref.name
	uploadURI, err := url.Parse(fmt.Sprintf("/v2/%s/blobs/uploads/", name))

	if err != nil {
		return nil, errors.Wrap(err, "Failed to parse upload URL")
	}

	uploadURI = o.url.ResolveReference(uploadURI)
	req, err := http.NewRequest("POST", uploadURI.String(), nil)

	if err != nil {
		return nil, errors.Wrap(err, "Failed opening POST request")
	}

	if o.authCreds != "" {
		req.Header.Add("Authorization", "Basic "+o.authCreds)
	}

	resp, err := o.client.Do(req)

	if err != nil {
		return nil, errors.Wrapf(err, "Failed posting request %v", req)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("server returned an error %d", resp.StatusCode)
	}

	_, err = ioutil.ReadAll(resp.Body)

	if err != nil {
		return nil, errors.Wrapf(err, "Error reading response body for %s", name)
	}

	session, err := resp.Location()

	if err != nil {
		return nil, errors.Wrap(err, "Failed decoding response")
	}

	return o.url.ResolveReference(session), nil
}

// @path is the uuid upload path returned by the server to our Post request.
// @stream is the data source for the layer.
// Return the digest and size of the layer that was uploaded.
//nolint (gocognit)
func (o *OciRepo) CompleteLayer(session *url.URL, stream io.Reader) (digest.Digest, int64, error) {
	digester := sha256.New()
	hashReader := io.TeeReader(stream, digester)
	// using "chunked" upload
	count := int64(0)
	for {
		const maxSize = 10 * 1024 * 1024
		var buf bytes.Buffer
		size, err := io.CopyN(&buf, hashReader, maxSize)
		if size == 0 {
			if err != io.EOF {
				return "", -1, errors.Wrapf(err, "Failed to copy stream")
			}
			break
		}
		req, err := http.NewRequest(http.MethodPatch, session.String(), &buf)
		if err != nil {
			return "", -1, errors.Wrap(err, "Failed opening Patch request")
		}
		if o.authCreds != "" {
			req.Header.Add("Authorization", "Basic "+o.authCreds)
		}

		req.ContentLength = size
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("Content-Range", fmt.Sprintf("%d-%d", count, count+size-1))
		resp, err := o.client.Do(req)
		if err != nil {
			return "", -1, errors.Wrapf(err, "Failed posting request %v", req)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted {
			return "", -1, fmt.Errorf("Server returned an error %d", resp.StatusCode)
		}
		count += size
		session, err = resp.Location()
		if err != nil {
			return "", -1, errors.Wrap(err, "Failed decoding response")
		}

		session = o.url.ResolveReference(session)
	}

	ourDigest := fmt.Sprintf("%x", digester.Sum(nil))
	d := digest.NewDigestFromEncoded(digest.SHA256, ourDigest)
	q := session.Query()
	q.Set("digest", d.String())
	session.RawQuery = q.Encode()
	req, err := http.NewRequest(http.MethodPut, session.String(), nil)
	if err != nil {
		return "", -1, errors.Wrap(err, "Failed opening Put request")
	}
	if o.authCreds != "" {
		req.Header.Add("Authorization", "Basic "+o.authCreds)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	putResp, err := o.client.Do(req)
	if err != nil {
		return "", -1, errors.Wrapf(err, "Failed putting request %v", req)
	}
	defer putResp.Body.Close()
	if putResp.StatusCode != http.StatusCreated {
		return "", -1, fmt.Errorf("Server returned an error %d", putResp.StatusCode)
	}

	servDigest, ok := putResp.Header["Docker-Content-Digest"]
	if !ok || len(servDigest) != 1 {
		return "", -1, fmt.Errorf("Server returned incomplete headers")
	}

	blobLoc, err := putResp.Location()
	if err != nil {
		return "", -1, errors.Wrap(err, "Failed decoding response")
	}

	blobLoc = o.url.ResolveReference(blobLoc)
	req, err = http.NewRequest("HEAD", blobLoc.String(), nil)
	if err != nil {
		return "", -1, errors.Wrap(err, "Failed opening Head request")
	}
	if o.authCreds != "" {
		req.Header.Add("Authorization", "Basic "+o.authCreds)
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return "", -1, errors.Wrapf(err, "Failed getting new layer %v", blobLoc)
	}

	Length, ok := resp.Header["Content-Length"]
	if !ok || len(Length) != 1 {
		return "", -1, fmt.Errorf("Server returned incomplete headers")
	}
	length, err := strconv.ParseInt(Length[0], 10, 64)
	if err != nil {
		return "", -1, errors.Wrap(err, "Failed decoding length in response")
	}

	if servDigest[0] != d.String() {
		return "", -1, errors.Wrapf(err, "Server calculated digest %s, not our %s", servDigest[0], ourDigest)
	}

	// TODO dist is returning the wrong thing - the hash,
	// not the "digest", which is "sha256:hash"

	return d, length, nil
}

// ociCertDir returns a path to a directory to be consumed by
// tlsclientconfig.SetupCertificates() depending on ctx and hostPort.
func ociCertDir(sys *types.SystemContext, hostPort string) (string, error) {
	if sys != nil && sys.DockerCertPath != "" {
		return sys.DockerCertPath, nil
	}

	if sys != nil && sys.DockerPerHostCertDirPath != "" {
		return filepath.Join(sys.DockerPerHostCertDirPath, hostPort), nil
	}

	var (
		hostCertDir               string
		fullCertDirPath           string
		systemPerHostCertDirPaths = [1]string{"/etc/containers/certs.d"}
	)

	for _, systemPerHostCertDirPath := range systemPerHostCertDirPaths {
		if sys != nil && sys.RootForImplicitAbsolutePaths != "" {
			hostCertDir = filepath.Join(sys.RootForImplicitAbsolutePaths, systemPerHostCertDirPath)
		} else {
			hostCertDir = systemPerHostCertDirPath
		}

		fullCertDirPath = filepath.Join(hostCertDir, hostPort)
		_, err := os.Stat(fullCertDirPath)

		if err == nil {
			break
		}

		if os.IsNotExist(err) {
			continue
		}

		if os.IsPermission(err) {
			continue
		}

		if err != nil {
			return "", err
		}
	}

	return fullCertDirPath, nil
}

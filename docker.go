package main

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/pkg/homedir"
	"github.com/projectatomic/skopeo/reference"
	"github.com/projectatomic/skopeo/types"
)

const (
	dockerHostname     = "docker.io"
	dockerRegistry     = "registry-1.docker.io"
	dockerAuthRegistry = "https://index.docker.io/v1/"

	dockerCfg         = ".docker"
	dockerCfgFileName = "config.json"
	dockerCfgObsolete = ".dockercfg"

	baseURL     = "%s://%s/v2/"
	tagsURL     = baseURL + "%s/tags/list"
	manifestURL = baseURL + "%s/manifests/%s"
	blobsURL    = baseURL + "%s/blobs/%s"
)

var (
	validHex = regexp.MustCompile(`^([a-f0-9]{64})$`)
)

type errFetchManifest struct {
	statusCode int
	body       []byte
}

func (e errFetchManifest) Error() string {
	return fmt.Sprintf("error fetching manifest: status code: %d, body: %s", e.statusCode, string(e.body))
}

type dockerImage struct {
	ref             reference.Named
	tag             string
	digest          string
	registry        string
	username        string
	password        string
	WWWAuthenticate string
	scheme          string
	rawManifest     []byte
	transport       *http.Transport
}

func (i *dockerImage) RawManifest(version string) ([]byte, error) {
	// TODO(runcom): unused version param for now, default to docker v2-1
	if err := i.retrieveRawManifest(); err != nil {
		return nil, err
	}
	return i.rawManifest, nil
}

func (i *dockerImage) Manifest() (types.ImageManifest, error) {
	// TODO(runcom): unused version param for now, default to docker v2-1
	m, err := i.getSchema1Manifest()
	if err != nil {
		return nil, err
	}
	ms1, ok := m.(*manifestSchema1)
	if !ok {
		return nil, fmt.Errorf("error retrivieng manifest schema1")
	}
	tags, err := i.getTags()
	if err != nil {
		return nil, err
	}
	imgManifest, err := makeImageManifest(i.ref.FullName(), ms1, i.digest, tags)
	if err != nil {
		return nil, err
	}
	return imgManifest, nil
}

func (i *dockerImage) getTags() ([]string, error) {
	url := fmt.Sprintf(tagsURL, i.scheme, i.registry, i.ref.RemoteName())
	res, err := i.makeRequest("GET", url, i.WWWAuthenticate != "", nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		// print url also
		return nil, fmt.Errorf("Invalid status code returned when fetching tags list %d", res.StatusCode)
	}
	type tagsRes struct {
		Tags []string
	}
	tags := &tagsRes{}
	if err := json.NewDecoder(res.Body).Decode(tags); err != nil {
		return nil, err
	}
	return tags.Tags, nil
}

type config struct {
	Labels map[string]string
}

type v1Image struct {
	// Config is the configuration of the container received from the client
	Config *config `json:"config,omitempty"`
	// DockerVersion specifies version on which image is built
	DockerVersion string `json:"docker_version,omitempty"`
	// Created timestamp when image was created
	Created time.Time `json:"created"`
	// Architecture is the hardware that the image is build and runs on
	Architecture string `json:"architecture,omitempty"`
	// OS is the operating system used to build and run the image
	OS string `json:"os,omitempty"`
}

func makeImageManifest(name string, m *manifestSchema1, dgst string, tagList []string) (types.ImageManifest, error) {
	v1 := &v1Image{}
	if err := json.Unmarshal([]byte(m.History[0].V1Compatibility), v1); err != nil {
		return nil, err
	}
	return &types.DockerImageManifest{
		Name:          name,
		Tag:           m.Tag,
		Digest:        dgst,
		RepoTags:      tagList,
		DockerVersion: v1.DockerVersion,
		Created:       v1.Created,
		Labels:        v1.Config.Labels,
		Architecture:  v1.Architecture,
		Os:            v1.OS,
		Layers:        m.GetLayers(),
	}, nil
}

// TODO(runcom)
func (i *dockerImage) DockerTar() ([]byte, error) {
	return nil, nil
}

// will support v1 one day...
type manifest interface {
	String() string
	GetLayers() []string
}

type manifestSchema1 struct {
	Name     string
	Tag      string
	FSLayers []struct {
		BlobSum string `json:"blobSum"`
	} `json:"fsLayers"`
	History []struct {
		V1Compatibility string `json:"v1Compatibility"`
	} `json:"history"`
	// TODO(runcom) verify the downloaded manifest
	//Signature []byte `json:"signature"`
}

func (m *manifestSchema1) GetLayers() []string {
	layers := make([]string, len(m.FSLayers))
	for i, layer := range m.FSLayers {
		layers[i] = layer.BlobSum
	}
	return layers
}

func (m *manifestSchema1) String() string {
	return fmt.Sprintf("%s-%s", sanitize(m.Name), sanitize(m.Tag))
}

func sanitize(s string) string {
	return strings.Replace(s, "/", "-", -1)
}

func (i *dockerImage) makeRequest(method, url string, auth bool, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Docker-Distribution-API-Version", "registry/2.0")
	for n, h := range headers {
		req.Header.Add(n, h)
	}
	if auth {
		if err := i.setupRequestAuth(req); err != nil {
			return nil, err
		}
	}
	client := &http.Client{}
	if i.transport != nil {
		client.Transport = i.transport
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (i *dockerImage) setupRequestAuth(req *http.Request) error {
	tokens := strings.SplitN(strings.TrimSpace(i.WWWAuthenticate), " ", 2)
	if len(tokens) != 2 {
		return fmt.Errorf("expected 2 tokens in WWW-Authenticate: %d, %s", len(tokens), i.WWWAuthenticate)
	}
	switch tokens[0] {
	case "Basic":
		req.SetBasicAuth(i.username, i.password)
		return nil
	case "Bearer":
		client := &http.Client{}
		if i.transport != nil {
			client.Transport = i.transport
		}
		res, err := client.Do(req)
		if err != nil {
			return err
		}
		hdr := res.Header.Get("WWW-Authenticate")
		if hdr == "" || res.StatusCode != http.StatusUnauthorized {
			// no need for bearer? wtf?
			return nil
		}
		tokens = strings.Split(hdr, " ")
		tokens = strings.Split(tokens[1], ",")
		var realm, service, scope string
		for _, token := range tokens {
			if strings.HasPrefix(token, "realm") {
				realm = strings.Trim(token[len("realm="):], "\"")
			}
			if strings.HasPrefix(token, "service") {
				service = strings.Trim(token[len("service="):], "\"")
			}
			if strings.HasPrefix(token, "scope") {
				scope = strings.Trim(token[len("scope="):], "\"")
			}
		}

		if realm == "" {
			return fmt.Errorf("missing realm in bearer auth challenge")
		}
		if service == "" {
			return fmt.Errorf("missing service in bearer auth challenge")
		}
		// The scope can be empty if we're not getting a token for a specific repo
		//if scope == "" && repo != "" {
		if scope == "" {
			return fmt.Errorf("missing scope in bearer auth challenge")
		}
		token, err := i.getBearerToken(realm, service, scope)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
		return nil
	}
	return fmt.Errorf("no handler for %s authentication", tokens[0])
	// support docker bearer with authconfig's Auth string? see docker2aci
}

func (i *dockerImage) getBearerToken(realm, service, scope string) (string, error) {
	authReq, err := http.NewRequest("GET", realm, nil)
	if err != nil {
		return "", err
	}
	getParams := authReq.URL.Query()
	getParams.Add("service", service)
	if scope != "" {
		getParams.Add("scope", scope)
	}
	authReq.URL.RawQuery = getParams.Encode()
	if i.username != "" && i.password != "" {
		authReq.SetBasicAuth(i.username, i.password)
	}
	// insecure for now to contact the external token service
	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	client := &http.Client{Transport: tr}
	res, err := client.Do(authReq)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusUnauthorized:
		return "", fmt.Errorf("unable to retrieve auth token: 401 unauthorized")
	case http.StatusOK:
		break
	default:
		return "", fmt.Errorf("unexpected http code: %d, URL: %s", res.StatusCode, authReq.URL)
	}
	tokenBlob, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	tokenStruct := struct {
		Token string `json:"token"`
	}{}
	if err := json.Unmarshal(tokenBlob, &tokenStruct); err != nil {
		return "", err
	}
	// TODO(runcom): reuse tokens?
	//hostAuthTokens, ok = rb.hostsV2AuthTokens[req.URL.Host]
	//if !ok {
	//hostAuthTokens = make(map[string]string)
	//rb.hostsV2AuthTokens[req.URL.Host] = hostAuthTokens
	//}
	//hostAuthTokens[repo] = tokenStruct.Token
	return tokenStruct.Token, nil
}

func (i *dockerImage) retrieveRawManifest() error {
	if i.rawManifest != nil {
		return nil
	}
	pr, err := i.ping()
	if err != nil {
		return err
	}
	i.WWWAuthenticate = pr.WWWAuthenticate
	i.scheme = pr.scheme
	url := fmt.Sprintf(manifestURL, i.scheme, i.registry, i.ref.RemoteName(), i.tag)
	// TODO(runcom) set manifest version header! schema1 for now - then schema2 etc etc and v1
	// TODO(runcom) NO, switch on the resulter manifest like Docker is doing
	res, err := i.makeRequest("GET", url, pr.needsAuth(), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	manblob, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusOK {
		return errFetchManifest{res.StatusCode, manblob}
	}
	i.rawManifest = manblob
	i.digest = res.Header.Get("Docker-Content-Digest")
	return nil
}

func (i *dockerImage) getSchema1Manifest() (manifest, error) {
	if err := i.retrieveRawManifest(); err != nil {
		return nil, err
	}
	mschema1 := &manifestSchema1{}
	if err := json.Unmarshal(i.rawManifest, mschema1); err != nil {
		return nil, err
	}
	if err := fixManifestLayers(mschema1); err != nil {
		return nil, err
	}
	// TODO(runcom): verify manifest schema 1, 2 etc
	//if len(m.FSLayers) != len(m.History) {
	//return nil, fmt.Errorf("length of history not equal to number of layers for %q", ref.String())
	//}
	//if len(m.FSLayers) == 0 {
	//return nil, fmt.Errorf("no FSLayers in manifest for %q", ref.String())
	//}
	return mschema1, nil
}

func (i *dockerImage) Layers(layers ...string) error {
	m, err := i.getSchema1Manifest()
	if err != nil {
		return err
	}
	tmpDir, err := ioutil.TempDir(".", "layers-"+m.String()+"-")
	if err != nil {
		return err
	}
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(path.Join(tmpDir, "manifest.json"), data, 0644); err != nil {
		return err
	}
	if len(layers) == 0 {
		layers = m.GetLayers()
	}
	for _, l := range layers {
		if !strings.HasPrefix(l, "sha256:") {
			l = "sha256:" + l
		}
		url := fmt.Sprintf(blobsURL, i.scheme, i.registry, i.ref.RemoteName(), l)
		if err := i.getLayer(l, url, tmpDir); err != nil {
			return err
		}
	}
	return nil
}

func (i *dockerImage) getLayer(l, url, tmpDir string) error {
	logrus.Infof("Downloading %s", url)
	res, err := i.makeRequest("GET", url, i.WWWAuthenticate != "", nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		// print url also
		return fmt.Errorf("Invalid status code returned when fetching blob %d", res.StatusCode)
	}
	layerPath := path.Join(tmpDir, strings.Replace(l, "sha256:", "", -1)+".tar")
	layerFile, err := os.Create(layerPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(layerFile, res.Body); err != nil {
		return err
	}
	if err := layerFile.Sync(); err != nil {
		return err
	}
	return nil
}

func parseDockerImage(img, certPath string, tlsVerify bool) (types.Image, error) {
	ref, err := reference.ParseNamed(img)
	if err != nil {
		return nil, err
	}
	if reference.IsNameOnly(ref) {
		ref = reference.WithDefaultTag(ref)
	}
	var tag string
	switch x := ref.(type) {
	case reference.Canonical:
		tag = x.Digest().String()
	case reference.NamedTagged:
		tag = x.Tag()
	}
	var registry string
	hostname := ref.Hostname()
	if hostname == dockerHostname {
		registry = dockerRegistry
	} else {
		registry = hostname
	}
	username, password, err := getAuth(ref.Hostname())
	if err != nil {
		return nil, err
	}
	var tr *http.Transport
	if certPath != "" {
		tlsc := &tls.Config{}

		cert, err := tls.LoadX509KeyPair(filepath.Join(certPath, "cert.pem"), filepath.Join(certPath, "key.pem"))
		if err != nil {
			return nil, fmt.Errorf("Error loading x509 key pair: %s", err)
		}

		tlsc.Certificates = append(tlsc.Certificates, cert)
		tlsc.InsecureSkipVerify = !tlsVerify
		tr = &http.Transport{
			TLSClientConfig: tlsc,
		}
	}
	return &dockerImage{
		ref:       ref,
		tag:       tag,
		registry:  registry,
		username:  username,
		password:  password,
		transport: tr,
	}, nil
}

func getDefaultConfigDir(confPath string) string {
	return filepath.Join(homedir.Get(), confPath)
}

type dockerAuthConfigObsolete struct {
	Auth string `json:"auth"`
}

type dockerAuthConfig struct {
	Auth string `json:"auth,omitempty"`
}

type dockerConfigFile struct {
	AuthConfigs map[string]dockerAuthConfig `json:"auths"`
}

func decodeDockerAuth(s string) (string, string, error) {
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", "", err
	}
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		// if it's invalid just skip, as docker does
		return "", "", nil
	}
	user := parts[0]
	password := strings.Trim(parts[1], "\x00")
	return user, password, nil
}

func getAuth(hostname string) (string, string, error) {
	// TODO(runcom): get this from *cli.Context somehow
	//if username != "" && password != "" {
	//return username, password, nil
	//}
	if hostname == dockerHostname {
		hostname = dockerAuthRegistry
	}
	dockerCfgPath := filepath.Join(getDefaultConfigDir(".docker"), dockerCfgFileName)
	if _, err := os.Stat(dockerCfgPath); err == nil {
		j, err := ioutil.ReadFile(dockerCfgPath)
		if err != nil {
			return "", "", err
		}
		var dockerAuth dockerConfigFile
		if err := json.Unmarshal(j, &dockerAuth); err != nil {
			return "", "", err
		}
		// try the normal case
		if c, ok := dockerAuth.AuthConfigs[hostname]; ok {
			return decodeDockerAuth(c.Auth)
		}
	} else if os.IsNotExist(err) {
		oldDockerCfgPath := filepath.Join(getDefaultConfigDir(dockerCfgObsolete))
		if _, err := os.Stat(oldDockerCfgPath); err != nil {
			return "", "", nil //missing file is not an error
		}
		j, err := ioutil.ReadFile(oldDockerCfgPath)
		if err != nil {
			return "", "", err
		}
		var dockerAuthOld map[string]dockerAuthConfigObsolete
		if err := json.Unmarshal(j, &dockerAuthOld); err != nil {
			return "", "", err
		}
		if c, ok := dockerAuthOld[hostname]; ok {
			return decodeDockerAuth(c.Auth)
		}
	} else {
		// if file is there but we can't stat it for any reason other
		// than it doesn't exist then stop
		return "", "", fmt.Errorf("%s - %v", dockerCfgPath, err)
	}
	return "", "", nil
}

type apiErr struct {
	Code    string
	Message string
	Detail  interface{}
}

type pingResponse struct {
	WWWAuthenticate string
	APIVersion      string
	scheme          string
	errors          []apiErr
}

func (pr *pingResponse) needsAuth() bool {
	return pr.WWWAuthenticate != ""
}

func (i *dockerImage) ping() (*pingResponse, error) {
	client := &http.Client{}
	if i.transport != nil {
		client.Transport = i.transport
	}
	ping := func(scheme string) (*pingResponse, error) {
		url := fmt.Sprintf(baseURL, scheme, i.registry)
		resp, err := client.Get(url)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusUnauthorized {
			return nil, fmt.Errorf("error pinging repository, response code %d", resp.StatusCode)
		}
		pr := &pingResponse{}
		pr.WWWAuthenticate = resp.Header.Get("WWW-Authenticate")
		pr.APIVersion = resp.Header.Get("Docker-Distribution-Api-Version")
		pr.scheme = scheme
		if resp.StatusCode == http.StatusUnauthorized {
			type APIErrors struct {
				Errors []apiErr
			}
			errs := &APIErrors{}
			if err := json.NewDecoder(resp.Body).Decode(errs); err != nil {
				return nil, err
			}
			pr.errors = errs.Errors
		}
		return pr, nil
	}
	scheme := "https"
	pr, err := ping(scheme)
	if err != nil {
		scheme = "http"
		pr, err = ping(scheme)
		if err == nil {
			return pr, nil
		}
	}
	return pr, err
}

func fixManifestLayers(manifest *manifestSchema1) error {
	type imageV1 struct {
		ID     string
		Parent string
	}
	imgs := make([]*imageV1, len(manifest.FSLayers))
	for i := range manifest.FSLayers {
		img := &imageV1{}

		if err := json.Unmarshal([]byte(manifest.History[i].V1Compatibility), img); err != nil {
			return err
		}

		imgs[i] = img
		if err := validateV1ID(img.ID); err != nil {
			return err
		}
	}
	if imgs[len(imgs)-1].Parent != "" {
		return errors.New("Invalid parent ID in the base layer of the image.")
	}
	// check general duplicates to error instead of a deadlock
	idmap := make(map[string]struct{})
	var lastID string
	for _, img := range imgs {
		// skip IDs that appear after each other, we handle those later
		if _, exists := idmap[img.ID]; img.ID != lastID && exists {
			return fmt.Errorf("ID %+v appears multiple times in manifest", img.ID)
		}
		lastID = img.ID
		idmap[lastID] = struct{}{}
	}
	// backwards loop so that we keep the remaining indexes after removing items
	for i := len(imgs) - 2; i >= 0; i-- {
		if imgs[i].ID == imgs[i+1].ID { // repeated ID. remove and continue
			manifest.FSLayers = append(manifest.FSLayers[:i], manifest.FSLayers[i+1:]...)
			manifest.History = append(manifest.History[:i], manifest.History[i+1:]...)
		} else if imgs[i].Parent != imgs[i+1].ID {
			return fmt.Errorf("Invalid parent ID. Expected %v, got %v.", imgs[i+1].ID, imgs[i].Parent)
		}
	}
	return nil
}

func validateV1ID(id string) error {
	if ok := validHex.MatchString(id); !ok {
		return fmt.Errorf("image ID %q is invalid", id)
	}
	return nil
}

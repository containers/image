package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/docker/distribution/digest"
	"github.com/docker/docker/api"
	"github.com/docker/docker/image"
	"github.com/docker/docker/opts"
	versionPkg "github.com/docker/docker/pkg/version"
	"github.com/docker/docker/reference"
	"github.com/docker/docker/registry"
	types "github.com/docker/engine-api/types"
	containerTypes "github.com/docker/engine-api/types/container"
	"golang.org/x/net/context"
)

// fallbackError wraps an error that can possibly allow fallback to a different
// endpoint.
type fallbackError struct {
	// err is the error being wrapped.
	err error
	// confirmedV2 is set to true if it was confirmed that the registry
	// supports the v2 protocol. This is used to limit fallbacks to the v1
	// protocol.
	confirmedV2 bool
}

// Error renders the FallbackError as a string.
func (f fallbackError) Error() string {
	return f.err.Error()
}

type manifestFetcher interface {
	Fetch(ctx context.Context, ref reference.Named) (*imageInspect, error)
}

type imageInspect struct {
	V1ID            string `json:"V1Id"`
	RepoTags        []string
	RepoDigests     []string
	Parent          string
	Comment         string
	Created         string
	Container       string
	ContainerConfig *containerTypes.Config
	DockerVersion   string
	Author          string
	Config          *containerTypes.Config
	Architecture    string
	Os              string
	Size            int64
	Registry        string
}

func inspect(c *cli.Context) (*imageInspect, error) {
	ref, err := reference.ParseNamed(c.Args().First())
	if err != nil {
		return nil, err
	}
	authConfig, err := getAuthConfig(c, ref)
	if err != nil {
		return nil, err
	}
	var (
		ii *imageInspect
	)
	// TODO(runcom): remove docker.io case cause unqualified images
	// can be from additional registry below
	// tweak the ParseNamed above so I can know if its unqualified
	if ref.Hostname() != "" {
		ii, err = getData(ref, authConfig)
		if err != nil {
			return nil, err
		}
		return ii, nil
	}

	// TODO(runcom): ...
	// both authConfig and unqualified images

	return nil, nil
}

func getData(ref reference.Named, authConfig types.AuthConfig) (*imageInspect, error) {
	repoInfo, err := registry.ParseRepositoryInfo(ref)
	if err != nil {
		return nil, err
	}
	if err := validateRepoName(repoInfo.Name()); err != nil {
		return nil, err
	}

	// FATA[0000] open /etc/docker/certs.d/myreg.com:4000: permission denied
	// need to be run as root, really? :(
	// just pass tlsconfig via cli?!?!?!
	//
	// this happens only with private registry, docker.io works out of the box
	// EDIT: this happens with v1 registries?! no
	//
	// TODO(runcom): do not assume docker is installed on the system!
	// just fallback as for getAuthConfig
	options := &registry.Options{}
	options.InsecureRegistries = opts.NewListOpts(nil)
	options.Mirrors = opts.NewListOpts(nil)
	options.InsecureRegistries.Set("0.0.0.0/0")
	registryService := registry.NewService(options)
	for _, ic := range registryService.Config.IndexConfigs {
		ic.Secure = false
	}

	endpoints, err := registryService.LookupPullEndpoints(repoInfo)
	if err != nil {
		return nil, err
	}

	var (
		ctx                    = context.Background()
		lastErr                error
		discardNoSupportErrors bool
		imgInspect             *imageInspect
		confirmedV2            bool
	)

	for _, endpoint := range endpoints {
		// TODO(runcom):
		//
		// always try to login first so the registry is pinged and we can return timeout
		// instead of trying every v version (like push,pull and others do in docker)
		//
		//./skopeo --debug --username runcom --password 20121990cia0@! myreg.com:4000/rhel7
		//DEBU[0000] hostDir: /etc/docker/certs.d/https:/index.docker.io/v1
		//FATA[0000] open /etc/docker/certs.d/https:/index.docker.io/v1: permission denied
		//
		//status, err := registryService.Auth(&authConfig)
		//if err != nil {
		//return nil, err
		//}
		//logrus.Debug(status)

		if confirmedV2 && endpoint.Version == registry.APIVersion1 {
			logrus.Debugf("Skipping v1 endpoint %s because v2 registry was detected", endpoint.URL)
			continue
		}
		logrus.Debugf("Trying to fetch image manifest of %s repository from %s %s", repoInfo.Name(), endpoint.URL, endpoint.Version)

		//fetcher, err := newManifestFetcher(endpoint, repoInfo, config)
		fetcher, err := newManifestFetcher(endpoint, repoInfo, authConfig, registryService)
		if err != nil {
			lastErr = err
			continue
		}

		if imgInspect, err = fetcher.Fetch(ctx, ref); err != nil {
			// Was this fetch cancelled? If so, don't try to fall back.
			fallback := false
			select {
			case <-ctx.Done():
			default:
				if fallbackErr, ok := err.(fallbackError); ok {
					fallback = true
					confirmedV2 = confirmedV2 || fallbackErr.confirmedV2
					err = fallbackErr.err
				}
			}
			if fallback {
				if _, ok := err.(registry.ErrNoSupport); !ok {
					// Because we found an error that's not ErrNoSupport, discard all subsequent ErrNoSupport errors.
					discardNoSupportErrors = true
					// save the current error
					lastErr = err
				} else if !discardNoSupportErrors {
					// Save the ErrNoSupport error, because it's either the first error or all encountered errors
					// were also ErrNoSupport errors.
					lastErr = err
				}
				continue
			}
			logrus.Debugf("Not continuing with error: %v", err)
			return nil, err
		}

		return imgInspect, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("no endpoints found for %s", ref.String())
	}

	return nil, lastErr
}

func newManifestFetcher(endpoint registry.APIEndpoint, repoInfo *registry.RepositoryInfo, authConfig types.AuthConfig, registryService *registry.Service) (manifestFetcher, error) {
	switch endpoint.Version {
	case registry.APIVersion2:
		return &v2ManifestFetcher{
			endpoint:   endpoint,
			authConfig: authConfig,
			service:    registryService,
			repoInfo:   repoInfo,
		}, nil
	case registry.APIVersion1:
		return &v1ManifestFetcher{
			endpoint:   endpoint,
			authConfig: authConfig,
			service:    registryService,
			repoInfo:   repoInfo,
		}, nil
	}
	return nil, fmt.Errorf("unknown version %d for registry %s", endpoint.Version, endpoint.URL)
}

func getAuthConfig(c *cli.Context, ref reference.Named) (types.AuthConfig, error) {

	// use docker/cliconfig
	// if no /.docker -> docker not installed fallback to require username|password
	// maybe prompt user:passwd?

	var (
		authConfig types.AuthConfig
		username   = c.GlobalString("username")
		password   = c.GlobalString("password")
	)
	if username != "" && password != "" {
		authConfig = types.AuthConfig{
			Username: username,
			Password: password,
			Email:    "antonio.murdaca@gmail.com",
		}
	}

	return authConfig, nil
}

func validateRepoName(name string) error {
	if name == "" {
		return fmt.Errorf("Repository name can't be empty")
	}
	if name == api.NoBaseImageSpecifier {
		return fmt.Errorf("'%s' is a reserved name", api.NoBaseImageSpecifier)
	}
	return nil
}

func makeImageInspect(repoInfo *registry.RepositoryInfo, img *image.Image, tag string, dgst digest.Digest) *imageInspect {
	var repoTags = make([]string, 0, 1)
	if tagged, isTagged := repoInfo.Named.(reference.NamedTagged); isTagged || tag != "" {
		if !isTagged {
			newTagged, err := reference.WithTag(repoInfo, tag)
			if err == nil {
				tagged = newTagged
			}
		}
		if tagged != nil {
			repoTags = append(repoTags, tagged.String())
		}
	}

	var repoDigests = make([]string, 0, 1)
	if err := dgst.Validate(); err == nil {
		repoDigests = append(repoDigests, dgst.String())
	}

	return &imageInspect{
		V1ID:            img.V1Image.ID,
		RepoTags:        repoTags,
		RepoDigests:     repoDigests,
		Parent:          img.Parent.String(),
		Comment:         img.Comment,
		Created:         img.Created.Format(time.RFC3339Nano),
		Container:       img.Container,
		ContainerConfig: &img.ContainerConfig,
		DockerVersion:   img.DockerVersion,
		Author:          img.Author,
		Config:          img.Config,
		Architecture:    img.Architecture,
		Os:              img.OS,
		Size:            img.Size,
		Registry:        repoInfo.Index.Name,
	}
}

func makeRawConfigFromV1Config(imageJSON []byte, rootfs *image.RootFS, history []image.History) (map[string]*json.RawMessage, error) {
	var dver struct {
		DockerVersion string `json:"docker_version"`
	}

	if err := json.Unmarshal(imageJSON, &dver); err != nil {
		return nil, err
	}

	useFallback := versionPkg.Version(dver.DockerVersion).LessThan("1.8.3")

	if useFallback {
		var v1Image image.V1Image
		err := json.Unmarshal(imageJSON, &v1Image)
		if err != nil {
			return nil, err
		}
		imageJSON, err = json.Marshal(v1Image)
		if err != nil {
			return nil, err
		}
	}

	var c map[string]*json.RawMessage
	if err := json.Unmarshal(imageJSON, &c); err != nil {
		return nil, err
	}

	c["rootfs"] = rawJSON(rootfs)
	c["history"] = rawJSON(history)

	return c, nil
}

func rawJSON(value interface{}) *json.RawMessage {
	jsonval, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return (*json.RawMessage)(&jsonval)
}

package main

import (
	"fmt"

	"github.com/codegangsta/cli"
	"github.com/docker/docker/api"
	"github.com/docker/docker/reference"
	"github.com/docker/docker/registry"
	engineTypes "github.com/docker/engine-api/types"
	containerTypes "github.com/docker/engine-api/types/container"
)

type imageInspect struct {
	// I shouldn't need json tag here...
	ID              string `json:"Id"`
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

	var (
		ii *imageInspect
	)

	if ref.Hostname() != "" {
		ii, err = getData(ref)
		if err != nil {
			return nil, err
		}
		return ii, nil
	}

	authConfig, err := getAuthConfig(c, ref)
	if err != nil {
		return nil, err
	}

	_ = authConfig

	return nil, nil
}

func getData(ref reference.Named) (*imageInspect, error) {
	repoInfo, err := registry.ParseRepositoryInfo(ref)
	if err != nil {
		return nil, err
	}
	if err := validateRepoName(repoInfo.Name()); err != nil {
		return nil, err
	}

	registryService := registry.NewService(nil)

	// FATA[0000] open /etc/docker/certs.d/myreg.com:4000: permission denied
	// need to be run as root, really? :(
	// just pass tlsconfig via cli?!?!?!
	//
	// TODO(runcom): do not assume docker is installed on the system!
	// just fallback as for getAuthConfig
	endpoints, err := registryService.LookupPullEndpoints(repoInfo)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func getAuthConfig(c *cli.Context, ref reference.Named) (engineTypes.AuthConfig, error) {

	// use docker/cliconfig
	// if no /.docker -> docker not installed fallback to require username|password
	// maybe prompt user:passwd?

	//var (
	//authConfig engineTypes.AuthConfig
	//username   = c.GlobalString("username")
	//password   = c.GlobalString("password")
	//)
	//if username != "" && password != "" {
	//authConfig = engineTypes.AuthConfig{
	//Username: username,
	//Password: password,
	//}
	//}

	return engineTypes.AuthConfig{}, nil
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

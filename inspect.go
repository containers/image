package main

import (
	"github.com/docker/docker/reference"
	engineTypes "github.com/docker/engine-api/types"
	containerTypes "github.com/docker/engine-api/types/container"
)

type ImageInspect struct {
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

func inspect(ref reference.Named, authConfig engineTypes.AuthConfig) (string, error) {
	return "", nil
}

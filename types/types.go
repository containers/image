package types

import (
	containerTypes "github.com/docker/engine-api/types/container"
)

type Kind int

const (
	KindUnknown Kind = iota
	KindDocker
	KindAppc

	DockerPrefix = "docker://"
)

type Image interface {
	Kind() Kind
	GetLayers(layers []string) error
	GetManifest(version string) ([]byte, error)
	GetRawManifest(version string) ([]byte, error)
}

type ImageManifest struct {
	Tag             string
	Digest          string
	RepoTags        []string
	Comment         string
	Created         string
	ContainerConfig *containerTypes.Config
	DockerVersion   string
	Author          string
	Config          *containerTypes.Config
	Architecture    string
	Os              string
}

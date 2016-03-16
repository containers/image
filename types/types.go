package types

import (
	containerTypes "github.com/docker/engine-api/types/container"
)

const (
	DockerPrefix = "docker://"
)

type Registry interface {
	Images() []Image
	Image(ref string) Image     // ref == image name w/o registry part
	Lookup(term string) []Image // docker registry v1 only AFAICT, v2 can be built hacking with Images()
}

type Image interface {
	Layers(layers []string) error // var args
	Manifest(version string) (ImageManifest, error)
	RawManifest(version string) ([]byte, error)
	DockerTar() ([]byte, error) // ???
}

type ImageManifest struct {
	Tag             string
	Digest          string
	RepoTags        []string
	Comment         string
	Created         string
	ContainerConfig *containerTypes.Config // remove docker/docker code
	DockerVersion   string
	Author          string
	Config          *containerTypes.Config // remove docker/docker code
	Architecture    string
	Os              string
	Layers          []string // ???
}

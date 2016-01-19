package main

import (
	"fmt"

	"github.com/docker/distribution"
	"github.com/docker/docker/reference"
	"github.com/docker/docker/registry"
	"golang.org/x/net/context"
)

type v2ManifestFetcher struct {
	endpoint    registry.APIEndpoint
	repoInfo    *registry.RepositoryInfo
	repo        distribution.Repository
	confirmedV2 bool
}

func (mf *v2ManifestFetcher) Fetch(ctx context.Context, ref reference.Named) (*imageInspect, error) {
	fmt.Println("ciaone")
	return nil, nil
}

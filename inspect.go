package main

import (
	"fmt"

	"github.com/codegangsta/cli"
	"github.com/runcom/skopeo/docker"
	"github.com/runcom/skopeo/types"
)

const (
	imgTypeDocker = "docker"
	imgTypeAppc   = "appc"
)

func inspect(c *cli.Context) (*types.ImageInspect, error) {
	var (
		imgInspect *types.ImageInspect
		err        error
		name       = c.Args().First()
		imgType    = c.GlobalString("img-type")
	)
	switch imgType {
	case imgTypeDocker:
		imgInspect, err = docker.GetData(c, name)
		if err != nil {
			return nil, err
		}
	case imgTypeAppc:
		return nil, fmt.Errorf("sorry, not implemented yet")
	default:
		return nil, fmt.Errorf("%s image type is invalid, please use either 'docker' or 'appc'", imgType)
	}
	return imgInspect, nil
}

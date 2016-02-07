package main

import (
	"fmt"

	"github.com/codegangsta/cli"
	"github.com/runcom/skopeo/docker"
	"github.com/runcom/skopeo/types"
)

func inspect(c *cli.Context) (*types.ImageInspect, error) {
	var (
		imgInspect *types.ImageInspect
		err        error
		name       = c.Args().First()
		imgType    = c.GlobalString("img-type")
	)
	switch imgType {
	case "docker":
		imgInspect, err = docker.GetData(c, name)
		if err != nil {
			return nil, err
		}
	case "appc":
		return nil, fmt.Errorf("TODO")
	default:
		return nil, fmt.Errorf("%s image type is invalid, please use either 'docker' or 'appc'", imgType)
	}
	return imgInspect, nil
}

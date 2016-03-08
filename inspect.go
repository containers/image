package main

import (
	"fmt"
	"strings"

	"github.com/codegangsta/cli"
	"github.com/projectatomic/skopeo/docker"
	"github.com/projectatomic/skopeo/types"
)

type imgKind int

const (
	imgTypeDocker = "docker://"
	imgTypeAppc   = "appc://"

	kindUnknown = iota
	kindDocker
	kindAppc
)

func getImgType(img string) imgKind {
	if strings.HasPrefix(img, imgTypeDocker) {
		return kindDocker
	}
	if strings.HasPrefix(img, imgTypeAppc) {
		return kindAppc
	}
	// TODO(runcom): v2 will support this
	//return kindUnknown
	return kindDocker
}

func inspect(c *cli.Context) (*types.ImageInspect, error) {
	var (
		imgInspect *types.ImageInspect
		err        error
		name       = c.Args().First()
		kind       = getImgType(name)
	)

	switch kind {
	case kindDocker:
		imgInspect, err = docker.GetData(c, strings.Replace(name, imgTypeDocker, "", -1))
		if err != nil {
			return nil, err
		}
	case kindAppc:
		return nil, fmt.Errorf("not implemented yet")
	default:
		return nil, fmt.Errorf("%s image is invalid, please use either 'docker://' or 'appc://'", name)
	}
	return imgInspect, nil
}

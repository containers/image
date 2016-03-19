package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/projectatomic/skopeo"
	pkgInspect "github.com/projectatomic/skopeo/docker/inspect"
	"github.com/projectatomic/skopeo/types"
)

var inspectCmd = cli.Command{
	Name:      "inspect",
	Usage:     "inspect images on a registry",
	ArgsUsage: ``,
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "raw",
			Usage: "output raw manifest",
		},
	},
	Action: func(c *cli.Context) {
		if c.Bool("raw") {
			img, err := skopeo.ParseImage(c.Args().First())
			if err != nil {
				logrus.Fatal(err)
			}
			// TODO(runcom): this is not falling back to v1
			// TODO(runcom): hardcoded schema 2 version 1
			b, err := img.RawManifest("2-1")
			if err != nil {
				logrus.Fatal(err)
			}
			fmt.Println(string(b))
			return
		}
		// get the Image interface before inspecting...utils.go parseImage
		imgInspect, err := inspect(c)
		if err != nil {
			logrus.Fatal(err)
		}
		out, err := json.Marshal(imgInspect)
		if err != nil {
			logrus.Fatal(err)
		}
		fmt.Println(string(out))
	},
}

func inspect(c *cli.Context) (types.ImageManifest, error) {
	var (
		imgInspect types.ImageManifest
		err        error
		name       = c.Args().First()
	)

	switch {
	case strings.HasPrefix(name, types.DockerPrefix):
		imgInspect, err = pkgInspect.GetData(c, strings.Replace(name, "docker://", "", -1))
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("%s image is invalid, please use 'docker://'", name)
	}
	return imgInspect, nil
}

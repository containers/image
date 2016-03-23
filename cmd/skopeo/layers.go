package main

import (
	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/projectatomic/skopeo"
)

// TODO(runcom): document args and usage
var layersCmd = cli.Command{
	Name:  "layers",
	Usage: "get images layers",
	Action: func(c *cli.Context) {
		img, err := skopeo.ParseImage(c)
		if err != nil {
			logrus.Fatal(err)
		}
		if err := img.Layers(c.Args().Tail()...); err != nil {
			logrus.Fatal(err)
		}
	},
}

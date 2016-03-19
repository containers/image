package main

import (
	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/projectatomic/skopeo"
)

// TODO(runcom): document args and usage
var layersCmd = cli.Command{
	Name:      "layers",
	Usage:     "get images layers",
	ArgsUsage: ``,
	Action: func(context *cli.Context) {
		img, err := skopeo.ParseImage(context.Args().First())
		if err != nil {
			logrus.Fatal(err)
		}
		if err := img.Layers(context.Args().Tail()...); err != nil {
			logrus.Fatal(err)
		}
	},
}

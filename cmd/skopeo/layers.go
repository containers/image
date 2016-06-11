package main

import (
	"github.com/urfave/cli"
)

// TODO(runcom): document args and usage
var layersCmd = cli.Command{
	Name:  "layers",
	Usage: "get images layers",
	Action: func(c *cli.Context) error {
		img, err := parseImage(c)
		if err != nil {
			return err
		}
		if err := img.LayersCommand(c.Args().Tail()...); err != nil {
			return err
		}
		return nil
	},
}

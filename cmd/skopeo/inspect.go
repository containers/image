package main

import (
	"encoding/json"
	"fmt"

	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
)

var inspectCmd = cli.Command{
	Name:  "inspect",
	Usage: "inspect images on a registry",
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "raw",
			Usage: "output raw manifest",
		},
	},
	Action: func(c *cli.Context) {
		img, err := parseImage(c)
		if err != nil {
			logrus.Fatal(err)
		}
		if c.Bool("raw") {
			b, err := img.Manifest()
			if err != nil {
				logrus.Fatal(err)
			}
			fmt.Println(string(b))
			return
		}
		imgInspect, err := img.Inspect()
		if err != nil {
			logrus.Fatal(err)
		}
		out, err := json.MarshalIndent(imgInspect, "", "    ")
		if err != nil {
			logrus.Fatal(err)
		}
		fmt.Println(string(out))
	},
}

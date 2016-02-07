package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/docker/docker/cliconfig"
)

const (
	version = "0.1.5-dev"
	usage   = "inspect images on a registry"
)

var inspectCmd = func(c *cli.Context) {
	imgInspect, err := inspect(c)
	if err != nil {
		logrus.Fatal(err)
	}
	out, err := json.Marshal(imgInspect)
	if err != nil {
		logrus.Fatal(err)
	}
	fmt.Println(string(out))
}

func main() {
	app := cli.NewApp()
	app.Name = "skopeo"
	app.Version = version
	app.Usage = usage
	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:  "debug",
			Usage: "enable debug output",
		},
		cli.StringFlag{
			Name:  "username",
			Value: "",
			Usage: "registry username",
		},
		cli.StringFlag{
			Name:  "password",
			Value: "",
			Usage: "registry password",
		},
		cli.StringFlag{
			Name:  "docker-cfg",
			Value: cliconfig.ConfigDir(),
			Usage: "Docker's cli config for auth",
		},
		cli.StringFlag{
			Name:  "img-type",
			Value: "",
			Usage: "Either docker or appc",
		},
	}
	app.Before = func(c *cli.Context) error {
		if c.GlobalBool("debug") {
			logrus.SetLevel(logrus.DebugLevel)
		}
		return nil
	}
	app.Action = inspectCmd
	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}

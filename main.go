package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
)

const (
	version = "0.0.1"
	usage   = "inspect images"
)

var inspectCmd = func(c *cli.Context) {
	imgInspect, err := inspect(c)
	if err != nil {
		logrus.Fatal(err)
	}
	data, err := json.Marshal(imgInspect)
	if err != nil {
		logrus.Fatal(err)
	}
	fmt.Println(string(data))
}

func main() {
	app := cli.NewApp()
	app.Name = "skopeo"
	app.Version = version
	app.Usage = usage
	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:  "debug",
			Usage: "enable debug output for logging",
		},
		//cli.BoolFlag{
		//Name: "tags",
		//Usage: "show tags"
		//},
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

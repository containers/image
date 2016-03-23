package main

import (
	"fmt"
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/projectatomic/skopeo"
)

func main() {
	app := cli.NewApp()
	app.Name = "skopeo"
	if skopeo.GitCommit != "" {
		app.Version = fmt.Sprintf("%s commit: %s", skopeo.Version, skopeo.GitCommit)
	} else {
		app.Version = skopeo.Version
	}
	app.Usage = "interact with registries"
	// TODO(runcom)
	//app.EnableBashCompletion = true
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
			Name:  "cert-path",
			Value: "",
			Usage: "Certificates path to connect to the given registry (cert.pem, key.pem)",
		},
		cli.BoolFlag{
			Name:  "tls-verify",
			Usage: "Whether to verify certificates or not",
		},
	}
	app.Before = func(c *cli.Context) error {
		if c.GlobalBool("debug") {
			logrus.SetLevel(logrus.DebugLevel)
		}
		return nil
	}
	app.Commands = []cli.Command{
		inspectCmd,
		layersCmd,
		standaloneSignCmd,
		standaloneVerifyCmd,
	}
	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}

package main

import (
	"fmt"
	"strings"

	"github.com/codegangsta/cli"
	"github.com/projectatomic/skopeo/types"
)

// ParseImage converts image URL-like string to an initialized handler for that image.
func parseImage(c *cli.Context) (types.Image, error) {
	var (
		imgName   = c.Args().First()
		certPath  = c.GlobalString("cert-path")
		tlsVerify = c.GlobalBool("tls-verify")
	)
	switch {
	case strings.HasPrefix(imgName, types.DockerPrefix):
		return parseDockerImage(strings.TrimPrefix(imgName, types.DockerPrefix), certPath, tlsVerify)
		//case strings.HasPrefix(img, appcPrefix):
		//
	}
	return nil, fmt.Errorf("no valid prefix provided")
}

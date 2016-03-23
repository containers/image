package skopeo

import (
	"fmt"
	"strings"

	"github.com/codegangsta/cli"
	"github.com/projectatomic/skopeo/types"
)

// ParseImage converts image URL-like string to an initialized handler for that image.
func ParseImage(c *cli.Context) (types.Image, error) {
	imgName := c.Args().First()
	switch {
	case strings.HasPrefix(imgName, types.DockerPrefix):
		return parseDockerImage(strings.TrimPrefix(imgName, types.DockerPrefix))
		//case strings.HasPrefix(img, appcPrefix):
		//
	}
	return nil, fmt.Errorf("no valid prefix provided")
}

package daemon

import (
	"github.com/containers/image/v5/types"
	dockerclient "github.com/fsouza/go-dockerclient"
)

// NewDockerClient initializes a new API client based on the passed SystemContext.
func newDockerClient(sys *types.SystemContext) (*dockerclient.Client, error) {
	host := "unix:///var/run/docker.sock"
	if sys != nil && sys.DockerDaemonHost != "" {
		host = sys.DockerDaemonHost
	}

	return dockerclient.NewClient(host)
}

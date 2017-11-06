package daemon

import (
	"github.com/containers/image/types"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/go-connections/tlsconfig"
	"net/http"
	"path/filepath"
)

const (
	// The default API version to be used in case none is explicitly specified
	defaultAPIVersion = "1.22"
)

// NewDockerClient initializes a new API client based on the passed SystemContext.
func newDockerClient(ctx *types.SystemContext) (*dockerclient.Client, error) {
	httpClient, err := tlsConfig(ctx)
	if err != nil {
		return nil, err
	}

	host := dockerclient.DefaultDockerHost
	if ctx != nil && ctx.DockerDaemonHost != "" {
		host = ctx.DockerDaemonHost
	}

	return dockerclient.NewClient(host, defaultAPIVersion, httpClient, nil)
}

func tlsConfig(ctx *types.SystemContext) (*http.Client, error) {
	options := tlsconfig.Options{}
	if ctx != nil && ctx.DockerDaemonInsecureSkipTLSVerify {
		options.InsecureSkipVerify = true
	}

	if ctx != nil && ctx.DockerDaemonCertPath != "" {
		options.CAFile = filepath.Join(ctx.DockerDaemonCertPath, "ca.pem")
		options.CertFile = filepath.Join(ctx.DockerDaemonCertPath, "cert.pem")
		options.KeyFile = filepath.Join(ctx.DockerDaemonCertPath, "key.pem")
	}

	tlsc, err := tlsconfig.Client(options)
	if err != nil {
		return nil, err
	}

	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsc,
		},
		CheckRedirect: dockerclient.CheckRedirect,
	}, nil
}

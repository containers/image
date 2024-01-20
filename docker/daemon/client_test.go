package daemon

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/containers/image/v5/types"
	dockerclient "github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
)

func TestDockerClientFromNilSystemContext(t *testing.T) {
	client, err := newDockerClient(context.Background(), nil)

	assert.Nil(t, err, "There should be no error creating the Docker client")
	assert.NotNil(t, client, "A Docker client reference should have been returned")

	assert.Equal(t, dockerclient.DefaultDockerHost, client.DaemonHost(), "The default docker host should have been used")

	clientVersionNumbers := strings.Split(client.ClientVersion(), ".")

	major, err := strconv.Atoi(clientVersionNumbers[0])
	assert.NoError(t, err, "The client major version should be a number")

	minor, err := strconv.Atoi(clientVersionNumbers[1])
	assert.NoError(t, err, "The client minor version should be a number")

	// The client defaults to 1.24 if negotiation fails.
	assert.True(t, major == 1 && minor > 24, client.ClientVersion(), "Should have successfully negotiated a client version")

	assert.NoError(t, client.Close())
}

func TestDockerClientFromCertContext(t *testing.T) {
	testDir := testDir(t)

	host := "tcp://127.0.0.1:2376"
	systemCtx := &types.SystemContext{
		DockerDaemonCertPath:              filepath.Join(testDir, "testdata", "certs"),
		DockerDaemonHost:                  host,
		DockerDaemonInsecureSkipTLSVerify: true,
	}

	client, err := newDockerClient(context.Background(), systemCtx)

	assert.Nil(t, err, "There should be no error creating the Docker client")
	assert.NotNil(t, client, "A Docker client reference should have been returned")

	assert.Equal(t, host, client.DaemonHost())

	clientVersionNumbers := strings.Split(client.ClientVersion(), ".")

	major, err := strconv.Atoi(clientVersionNumbers[0])
	assert.NoError(t, err, "The client major version should be a number")

	assert.Equal(t, 1, major, "The client major version should be 1")

	assert.NoError(t, client.Close())
}

func TestTlsConfigFromInvalidCertPath(t *testing.T) {
	ctx := &types.SystemContext{
		DockerDaemonCertPath: "/foo/bar",
	}

	_, err := tlsConfig(ctx)
	assert.ErrorContains(t, err, "could not read CA certificate")
}

func TestTlsConfigFromCertPath(t *testing.T) {
	testDir := testDir(t)

	ctx := &types.SystemContext{
		DockerDaemonCertPath:              filepath.Join(testDir, "testdata", "certs"),
		DockerDaemonInsecureSkipTLSVerify: true,
	}

	httpClient, err := tlsConfig(ctx)

	assert.NoError(t, err, "There should be no error creating the HTTP client")

	tlsConfig := httpClient.Transport.(*http.Transport).TLSClientConfig
	assert.True(t, tlsConfig.InsecureSkipVerify, "TLS verification should be skipped")
	assert.Len(t, tlsConfig.Certificates, 1, "There should be one certificate")
}

func TestSkipTLSVerifyOnly(t *testing.T) {
	//testDir := testDir(t)

	ctx := &types.SystemContext{
		DockerDaemonInsecureSkipTLSVerify: true,
	}

	httpClient, err := tlsConfig(ctx)

	assert.NoError(t, err, "There should be no error creating the HTTP client")

	tlsConfig := httpClient.Transport.(*http.Transport).TLSClientConfig
	assert.True(t, tlsConfig.InsecureSkipVerify, "TLS verification should be skipped")
	assert.Len(t, tlsConfig.Certificates, 0, "There should be no certificate")
}

func TestSpecifyPlainHTTPViaHostScheme(t *testing.T) {
	host := "http://127.0.0.1:2376"
	systemCtx := &types.SystemContext{
		DockerDaemonHost: host,
	}

	client, err := newDockerClient(context.Background(), systemCtx)

	assert.Nil(t, err, "There should be no error creating the Docker client")
	assert.NotNil(t, client, "A Docker client reference should have been returned")

	assert.Equal(t, host, client.DaemonHost())
	assert.NoError(t, client.Close())
}

func testDir(t *testing.T) string {
	testDir, err := os.Getwd()
	if err != nil {
		t.Fatal("Unable to determine the current test directory")
	}
	return testDir
}

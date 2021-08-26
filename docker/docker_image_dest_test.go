package docker

import (
	"bufio"
	"bytes"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsManifestInvalidError(t *testing.T) {
	// Sadly only a smoke test; this really should record all known errors exactly as they happen.

	// docker/distribution 2.1.1 when uploading to a tag (because it can’t find a matching tag
	// inside the manifest)
	response := "HTTP/1.1 400 Bad Request\r\n" +
		"Connection: close\r\n" +
		"Content-Length: 79\r\n" +
		"Content-Type: application/json; charset=utf-8\r\n" +
		"Date: Sat, 14 Aug 2021 19:27:29 GMT\r\n" +
		"Docker-Distribution-Api-Version: registry/2.0\r\n" +
		"\r\n" +
		"{\"errors\":[{\"code\":\"TAG_INVALID\",\"message\":\"manifest tag did not match URI\"}]}\n"
	resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader([]byte(response))), nil)
	require.NoError(t, err)
	err = registryHTTPResponseToError(resp)

	res := isManifestInvalidError(err)
	assert.True(t, res, "%#v", err)
}

package docker

import (
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSimplifyContentType(t *testing.T) {
	for _, c := range []struct{ input, expected string }{
		{"", ""},
		{"application/json", "application/json"},
		{"application/json;charset=utf-8", "application/json"},
		{"application/json; charset=utf-8", "application/json"},
		{"application/json ; charset=utf-8", "application/json"},
		{"application/json\t;\tcharset=utf-8", "application/json"},
		{"application/json    ;charset=utf-8", "application/json"},
		{`application/json; charset="utf-8"`, "application/json"},
		{"completely invalid", ""},
	} {
		out := simplifyContentType(c.input)
		assert.Equal(t, c.expected, out, c.input)
	}
}

type testClient struct{}

func (c *testClient) MakeRequest(method, url string, headers map[string][]string, stream io.Reader, resolved bool) (*http.Response, error) {
	return nil, nil
}

func TestIntendedDockerReference(t *testing.T) {
	client := &testClient{}

	img := "localhost:5000/runcom/busybox"
	src, err := NewImageSource(img, client)
	assert.NoError(t, err)
	expected := fmt.Sprintf("%s:latest", img)
	assert.Equal(t, expected, src.IntendedDockerReference())

	imgWithTag := "localhost:5000/runcom/busybox:amd64"
	src, err = NewImageSource(imgWithTag, client)
	assert.NoError(t, err)
	assert.Equal(t, imgWithTag, src.IntendedDockerReference())

	dockerImg := "runcom/busybox:amd64"
	src, err = NewImageSource(dockerImg, client)
	assert.NoError(t, err)
	assert.Equal(t, dockerImg, src.IntendedDockerReference())
}

package docker

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/containers/image/v5/internal/private"
	"github.com/containers/image/v5/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ private.ImageSource = (*dockerImageSource)(nil)

func TestDockerImageSourceReference(t *testing.T) {
	manifestPathRegex := regexp.MustCompile("^/v2/.*/manifests/latest$")

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v2/":
			rw.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && manifestPathRegex.MatchString(r.URL.Path):
			rw.WriteHeader(http.StatusOK)
			// Empty body is good enough for this test
		default:
			require.FailNowf(t, "Unexpected request", "%v %v", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	registryURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	registry := registryURL.Host

	mirrorConfiguration := strings.ReplaceAll(
		`[[registry]]
prefix = "primary-override.example.com"
location = "@REGISTRY@/primary-override"

[[registry]]
location = "with-mirror.example.com"

[[registry.mirror]]
location = "@REGISTRY@/with-mirror"
`, "@REGISTRY@", registry)
	registriesConf, err := os.CreateTemp("", "docker-image-src")
	require.NoError(t, err)
	defer registriesConf.Close()
	defer os.Remove(registriesConf.Name())
	err = os.WriteFile(registriesConf.Name(), []byte(mirrorConfiguration), 0600)
	require.NoError(t, err)

	for _, c := range []struct{ input, physical string }{
		{registry + "/no-redirection/busybox:latest", registry + "/no-redirection/busybox:latest"},
		{"primary-override.example.com/busybox:latest", registry + "/primary-override/busybox:latest"},
		{"with-mirror.example.com/busybox:latest", registry + "/with-mirror/busybox:latest"},
	} {
		ref, err := ParseReference("//" + c.input)
		require.NoError(t, err, c.input)
		src, err := ref.NewImageSource(context.Background(), &types.SystemContext{
			RegistriesDirPath:           "/this/does/not/exist",
			DockerPerHostCertDirPath:    "/this/does/not/exist",
			SystemRegistriesConfPath:    registriesConf.Name(),
			DockerInsecureSkipTLSVerify: types.OptionalBoolTrue,
		})
		require.NoError(t, err, c.input)
		defer src.Close()

		// The observable behavior
		assert.Equal(t, "//"+c.input, src.Reference().StringWithinTransport(), c.input)
		assert.Equal(t, ref.StringWithinTransport(), src.Reference().StringWithinTransport(), c.input)
		// Also peek into internal state
		src2, ok := src.(*dockerImageSource)
		require.True(t, ok, c.input)
		assert.Equal(t, "//"+c.input, src2.logicalRef.StringWithinTransport(), c.input)
		assert.Equal(t, "//"+c.physical, src2.physicalRef.StringWithinTransport(), c.input)
	}
}

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

func readNextStream(streams chan io.ReadCloser, errs chan error) ([]byte, error) {
	select {
	case r := <-streams:
		if r == nil {
			return nil, nil
		}
		defer r.Close()
		return io.ReadAll(r)
	case err := <-errs:
		return nil, err
	}
}

type verifyGetBlobAtData struct {
	expectedData  []byte
	expectedError error
}

func verifyGetBlobAtOutput(t *testing.T, streams chan io.ReadCloser, errs chan error, expected []verifyGetBlobAtData) {
	for _, c := range expected {
		data, err := readNextStream(streams, errs)
		assert.Equal(t, c.expectedData, data)
		assert.Equal(t, c.expectedError, err)
	}
}

func TestSplitHTTP200ResponseToPartial(t *testing.T) {
	body := io.NopCloser(bytes.NewReader([]byte("123456789")))
	defer body.Close()
	streams := make(chan io.ReadCloser)
	errs := make(chan error)
	chunks := []private.ImageSourceChunk{
		{Offset: 1, Length: 2},
		{Offset: 4, Length: 1},
	}
	go splitHTTP200ResponseToPartial(streams, errs, body, chunks)

	expected := []verifyGetBlobAtData{
		{[]byte("23"), nil},
		{[]byte("5"), nil},
		{[]byte(nil), nil},
	}

	verifyGetBlobAtOutput(t, streams, errs, expected)
}

func TestHandle206Response(t *testing.T) {
	body := io.NopCloser(bytes.NewReader([]byte("--AAA\r\n\r\n23\r\n--AAA\r\n\r\n5\r\n--AAA--")))
	defer body.Close()
	streams := make(chan io.ReadCloser)
	errs := make(chan error)
	chunks := []private.ImageSourceChunk{
		{Offset: 1, Length: 2},
		{Offset: 4, Length: 1},
	}
	mediaType := "multipart/form-data"
	params := map[string]string{
		"boundary": "AAA",
	}
	go handle206Response(streams, errs, body, chunks, mediaType, params)

	expected := []verifyGetBlobAtData{
		{[]byte("23"), nil},
		{[]byte("5"), nil},
		{[]byte(nil), nil},
	}
	verifyGetBlobAtOutput(t, streams, errs, expected)

	body = io.NopCloser(bytes.NewReader([]byte("HELLO")))
	defer body.Close()
	streams = make(chan io.ReadCloser)
	errs = make(chan error)
	chunks = []private.ImageSourceChunk{{Offset: 100, Length: 5}}
	mediaType = "text/plain"
	params = map[string]string{}
	go handle206Response(streams, errs, body, chunks, mediaType, params)

	expected = []verifyGetBlobAtData{
		{[]byte("HELLO"), nil},
		{[]byte(nil), nil},
	}
	verifyGetBlobAtOutput(t, streams, errs, expected)
}

func TestParseMediaType(t *testing.T) {
	mediaType, params, err := parseMediaType("multipart/byteranges; boundary=CloudFront:3F750DE0752BEDE3882F7DBE80010D31")
	require.NoError(t, err)
	assert.Equal(t, mediaType, "multipart/byteranges")
	assert.Equal(t, params["boundary"], "CloudFront:3F750DE0752BEDE3882F7DBE80010D31")

	mediaType, params, err = parseMediaType("multipart/byteranges; boundary=00000000000061573284")
	require.NoError(t, err)
	assert.Equal(t, mediaType, "multipart/byteranges")
	assert.Equal(t, params["boundary"], "00000000000061573284")

	mediaType, params, err = parseMediaType("multipart/byteranges; foo=bar; bar=baz")
	require.NoError(t, err)
	assert.Equal(t, mediaType, "multipart/byteranges")
	assert.Equal(t, params["foo"], "bar")
	assert.Equal(t, params["bar"], "baz")

	// quoted symbols '@'
	_, params, err = parseMediaType("multipart/byteranges; boundary=\"@:\"")
	require.NoError(t, err)
	assert.Equal(t, params["boundary"], "@:")

	// unquoted '@'
	_, _, err = parseMediaType("multipart/byteranges; boundary=@")
	require.Error(t, err)
}

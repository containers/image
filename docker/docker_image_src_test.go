package docker

// Many of these tests are made using github.com/dnaeon/go-vcr/recorder.
// See docker_client_test.go for more instructions.

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/containers/image/v5/types"
	"github.com/dnaeon/go-vcr/recorder"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	mirrorConfiguration := strings.Replace(
		`[[registry]]
prefix = "primary-override.example.com"
location = "@REGISTRY@/primary-override"

[[registry]]
location = "with-mirror.example.com"

[[registry.mirror]]
location = "@REGISTRY@/with-mirror"
`, "@REGISTRY@", registry, -1)
	registriesConf, err := ioutil.TempFile("", "docker-image-src")
	require.NoError(t, err)
	defer registriesConf.Close()
	defer os.Remove(registriesConf.Name())
	err = ioutil.WriteFile(registriesConf.Name(), []byte(mirrorConfiguration), 0600)
	require.NoError(t, err)

	for _, c := range []struct{ input, physical string }{
		{registry + "/no-redirection/busybox:latest", registry + "/no-redirection/busybox:latest"},
		{"primary-override.example.com/busybox:latest", registry + "/primary-override/busybox:latest"},
		{"with-mirror.example.com/busybox:latest", registry + "/with-mirror/busybox:latest"},
	} {
		ref, err := ParseReference("//" + c.input)
		require.NoError(t, err, c.input)
		src, err := ref.NewImageSource(context.Background(), &types.SystemContext{
			RegistriesDirPath:           "/this/doesnt/exist",
			DockerPerHostCertDirPath:    "/this/doesnt/exist",
			SystemRegistriesConfPath:    registriesConf.Name(),
			DockerInsecureSkipTLSVerify: types.OptionalBoolTrue,
		})
		require.NoError(t, err, c.input)

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

// TODO: TestNewImageSource
// TODO: TestDockerImageSourceReference
// TODO: TestDockerImageSourceClose

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

// vcrImageSource creates a dockerImageSource using a series of HTTP request/response recordings
// using recordingBaseName.
// It returns the imageSource or an error, and a cleanup callback
func vcrImageSource(t *testing.T, ctx *types.SystemContext, recordingBaseName string, mode recorder.Mode,
	ref string) (*dockerImageSource, func(), error) {
	ctx, httpWrapper, cleanup, dockerRef := prepareVCR(t, ctx, recordingBaseName, mode,
		ref)

	src, err := newImageSource(context.Background(), ctx, dockerRef, httpWrapper)
	return src, cleanup, err
}

func TestDockerImageSourceGetManifest(t *testing.T) {
	// Success
	src, cleanup, err := vcrImageSource(t, nil, "GetManifest-success", recorder.ModeReplaying,
		"//busybox:latest")
	defer cleanup()
	require.NoError(t, err)
	manifest, mimeType, err := src.GetManifest(context.Background(), nil)
	require.NoError(t, err)
	// Whatever was returned is now cached
	assert.Equal(t, src.cachedManifest, manifest)
	assert.Equal(t, src.cachedManifestMIMEType, mimeType)
	// TODO: Somehow test caching (i.e. that we don’t redo the request when doing it the second time)

	// Test failure fetching the manifest
	_, cleanup, err = vcrImageSource(t, nil, "GetManifest-not-found", recorder.ModeReplaying,
		"//busybox:this-does-not-exist")
	defer cleanup()
	assert.Error(t, err)
}

// TODO: Tests for quite a few methods.

// See the comment above TestDockerClientGetExtensionsSignatures for instructions on setting up the recording.
func TestDockerImageSourceGetSignaturesFromAPIExtension(t *testing.T) {
	ctx := &types.SystemContext{
		DockerAuthConfig: &types.DockerAuthConfig{
			Username: "unused",
			Password: "dh2juhu6LbGYGSHKMUa5BFEpyoPMYDVA59hxd3FCfbU",
		},
		DockerInsecureSkipTLSVerify: types.OptionalBoolTrue,
	}

	// Success
	// This only tests getting a single signature; the multiple-signature case
	// is tested within TestDockerImageDestinationPutSignaturesToAPIExtension.
	src, cleanup, err := vcrImageSource(t, ctx, "getSignaturesFromAPIExtension-success", recorder.ModeReplaying,
		"//localhost:5000/myns/personal:personal")
	defer cleanup()
	require.NoError(t, err)
	sigs, err := src.getSignaturesFromAPIExtension(context.Background(), nil)
	require.NoError(t, err)
	expectedSignature, err := ioutil.ReadFile("fixtures/extension-personal-personal.signature")
	require.NoError(t, err)
	assert.Equal(t, [][]byte{expectedSignature}, sigs)

	// TODO? Test that unknown signature kinds are silently ignored.
	// TODO? Test the various failure modes.
}

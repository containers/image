package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	internalManifest "github.com/containers/image/v5/internal/manifest"
	"github.com/containers/image/v5/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

const tokenResp = `{"token":"eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCIsIng1YyI6WyJNSUlDK1RDQ0FwK2dBd0lCQWdJQkFEQUtCZ2dxaGtqT1BRUURBakJHTVVRd1FnWURWUVFERXp0U1RVbEdPbEZNUmpRNlEwZFFNenBSTWtWYU9sRklSRUk2VkVkRlZUcFZTRlZNT2taTVZqUTZSMGRXV2pwQk5WUkhPbFJMTkZNNlVVeElTVEFlRncweU16QXhNRFl3TkRJM05EUmFGdzB5TkRBeE1qWXdOREkzTkRSYU1FWXhSREJDQmdOVkJBTVRPME5EVlVZNlNqVkhOanBGUTFORU9rTldSRWM2VkRkTU1qcEtXa1pST2xOTk0wUTZXRmxQTkRwV04wTkhPa2RHVjBJNldsbzFOam8wVlVSRE1JSUJJakFOQmdrcWhraUc5dzBCQVFFRkFBT0NBUThBTUlJQkNnS0NBUUVBek4wYjBqN1V5L2FzallYV2gyZzNxbzZKaE9rQWpYV0FVQmNzSHU2aFlaUkZMOXZlODEzVEI0Y2w4UWt4Q0k0Y1VnR0duR1dYVnhIMnU1dkV0eFNPcVdCcnhTTnJoU01qL1ZPKzYvaVkrOG1GRmEwR2J5czF3VDVjNlY5cWROaERiVGNwQXVYSjFSNGJLdSt1VGpVS0VIYXlqSFI5TFBEeUdnUC9ubUFadk5PWEdtclNTSkZJNnhFNmY3QS8rOVptcWgyVlRaQlc0cXduSnF0cnNJM2NveDNQczMwS2MrYUh3V3VZdk5RdFNBdytqVXhDVVFoRWZGa0lKSzh6OVdsL1FjdE9EcEdUeXNtVHBjNzZaVEdKWWtnaGhGTFJEMmJQTlFEOEU1ZWdKa2RQOXhpaW5sVGx3MjBxWlhVRmlqdWFBcndOR0xJbUJEWE0wWlI1YzVtU3Z3SURBUUFCbzRHeU1JR3ZNQTRHQTFVZER3RUIvd1FFQXdJSGdEQVBCZ05WSFNVRUNEQUdCZ1JWSFNVQU1FUUdBMVVkRGdROUJEdERRMVZHT2tvMVJ6WTZSVU5UUkRwRFZrUkhPbFEzVERJNlNscEdVVHBUVFRORU9saFpUelE2VmpkRFJ6cEhSbGRDT2xwYU5UWTZORlZFUXpCR0JnTlZIU01FUHpBOWdEdFNUVWxHT2xGTVJqUTZRMGRRTXpwUk1rVmFPbEZJUkVJNlZFZEZWVHBWU0ZWTU9rWk1WalE2UjBkV1dqcEJOVlJIT2xSTE5GTTZVVXhJU1RBS0JnZ3Foa2pPUFFRREFnTklBREJGQWlFQW1RNHhsQXZXVlArTy9hNlhDU05pYUFYRU1Bb1RQVFRYRWJYMks2RVU4ZTBDSUg0QTAwSVhtUndjdGtEOHlYNzdkTVoyK0pEY1FGdDFxRktMZFR5SnVzT1UiXX0.eyJhY2Nlc3MiOlt7InR5cGUiOiJyZXBvc2l0b3J5IiwibmFtZSI6ImxpYnJhcnkvYnVzeWJveCIsImFjdGlvbnMiOlsicHVsbCJdLCJwYXJhbWV0ZXJzIjp7InB1bGxfbGltaXQiOiIxMDAiLCJwdWxsX2xpbWl0X2ludGVydmFsIjoiMjE2MDAifX1dLCJhdWQiOiJyZWdpc3RyeS5kb2NrZXIuaW8iLCJleHAiOjE2ODU4MzYxMDEsImlhdCI6MTY4NTgzNTgwMSwiaXNzIjoiYXV0aC5kb2NrZXIuaW8iLCJqdGkiOiJkY2tyX2p0aV9QT0JjR0hLZEFTT3NBZkJfaWp1RWtKOGVTakE9IiwibmJmIjoxNjg1ODM1NTAxLCJzdWIiOiIifQ.DhHp7GB_JhKMr50EphQ9cxS1pOA-H4e9vd7zYCJqMj9DKvKsvhCGC3TFNefediYjgr3g8dz4bQC0Qr38Zg9yeSB8Kn7W7oG2NY-Cy12cHaKyW1X5Gn05Ho8ba6hxIB9ZS6lHMkgejyKKOjHNbcr74GBPqMS2ULfl4LMF9v3_hcyBifrGb0bnlESundr0_Vh0x0T0U27QRt7l-gGHfhaTwZUr-FtIpFhbZ08WQ_QovmaLziKIOp6nb9NuclnEGEwcz-yNHjLdb7qz9LQkCH53u40fWqfPYeTih7V2IcDPl4HozvNrELPibnjXvP72M4MhgC-knRgRscMUmZaTP8V_Mw","access_token":"eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCIsIng1YyI6WyJNSUlDK1RDQ0FwK2dBd0lCQWdJQkFEQUtCZ2dxaGtqT1BRUURBakJHTVVRd1FnWURWUVFERXp0U1RVbEdPbEZNUmpRNlEwZFFNenBSTWtWYU9sRklSRUk2VkVkRlZUcFZTRlZNT2taTVZqUTZSMGRXV2pwQk5WUkhPbFJMTkZNNlVVeElTVEFlRncweU16QXhNRFl3TkRJM05EUmFGdzB5TkRBeE1qWXdOREkzTkRSYU1FWXhSREJDQmdOVkJBTVRPME5EVlVZNlNqVkhOanBGUTFORU9rTldSRWM2VkRkTU1qcEtXa1pST2xOTk0wUTZXRmxQTkRwV04wTkhPa2RHVjBJNldsbzFOam8wVlVSRE1JSUJJakFOQmdrcWhraUc5dzBCQVFFRkFBT0NBUThBTUlJQkNnS0NBUUVBek4wYjBqN1V5L2FzallYV2gyZzNxbzZKaE9rQWpYV0FVQmNzSHU2aFlaUkZMOXZlODEzVEI0Y2w4UWt4Q0k0Y1VnR0duR1dYVnhIMnU1dkV0eFNPcVdCcnhTTnJoU01qL1ZPKzYvaVkrOG1GRmEwR2J5czF3VDVjNlY5cWROaERiVGNwQXVYSjFSNGJLdSt1VGpVS0VIYXlqSFI5TFBEeUdnUC9ubUFadk5PWEdtclNTSkZJNnhFNmY3QS8rOVptcWgyVlRaQlc0cXduSnF0cnNJM2NveDNQczMwS2MrYUh3V3VZdk5RdFNBdytqVXhDVVFoRWZGa0lKSzh6OVdsL1FjdE9EcEdUeXNtVHBjNzZaVEdKWWtnaGhGTFJEMmJQTlFEOEU1ZWdKa2RQOXhpaW5sVGx3MjBxWlhVRmlqdWFBcndOR0xJbUJEWE0wWlI1YzVtU3Z3SURBUUFCbzRHeU1JR3ZNQTRHQTFVZER3RUIvd1FFQXdJSGdEQVBCZ05WSFNVRUNEQUdCZ1JWSFNVQU1FUUdBMVVkRGdROUJEdERRMVZHT2tvMVJ6WTZSVU5UUkRwRFZrUkhPbFEzVERJNlNscEdVVHBUVFRORU9saFpUelE2VmpkRFJ6cEhSbGRDT2xwYU5UWTZORlZFUXpCR0JnTlZIU01FUHpBOWdEdFNUVWxHT2xGTVJqUTZRMGRRTXpwUk1rVmFPbEZJUkVJNlZFZEZWVHBWU0ZWTU9rWk1WalE2UjBkV1dqcEJOVlJIT2xSTE5GTTZVVXhJU1RBS0JnZ3Foa2pPUFFRREFnTklBREJGQWlFQW1RNHhsQXZXVlArTy9hNlhDU05pYUFYRU1Bb1RQVFRYRWJYMks2RVU4ZTBDSUg0QTAwSVhtUndjdGtEOHlYNzdkTVoyK0pEY1FGdDFxRktMZFR5SnVzT1UiXX0.eyJhY2Nlc3MiOlt7InR5cGUiOiJyZXBvc2l0b3J5IiwibmFtZSI6ImxpYnJhcnkvYnVzeWJveCIsImFjdGlvbnMiOlsicHVsbCJdLCJwYXJhbWV0ZXJzIjp7InB1bGxfbGltaXQiOiIxMDAiLCJwdWxsX2xpbWl0X2ludGVydmFsIjoiMjE2MDAifX1dLCJhdWQiOiJyZWdpc3RyeS5kb2NrZXIuaW8iLCJleHAiOjE2ODU4MzYxMDEsImlhdCI6MTY4NTgzNTgwMSwiaXNzIjoiYXV0aC5kb2NrZXIuaW8iLCJqdGkiOiJkY2tyX2p0aV9QT0JjR0hLZEFTT3NBZkJfaWp1RWtKOGVTakE9IiwibmJmIjoxNjg1ODM1NTAxLCJzdWIiOiIifQ.DhHp7GB_JhKMr50EphQ9cxS1pOA-H4e9vd7zYCJqMj9DKvKsvhCGC3TFNefediYjgr3g8dz4bQC0Qr38Zg9yeSB8Kn7W7oG2NY-Cy12cHaKyW1X5Gn05Ho8ba6hxIB9ZS6lHMkgejyKKOjHNbcr74GBPqMS2ULfl4LMF9v3_hcyBifrGb0bnlESundr0_Vh0x0T0U27QRt7l-gGHfhaTwZUr-FtIpFhbZ08WQ_QovmaLziKIOp6nb9NuclnEGEwcz-yNHjLdb7qz9LQkCH53u40fWqfPYeTih7V2IcDPl4HozvNrELPibnjXvP72M4MhgC-knRgRscMUmZaTP8V_Mw","expires_in":300,"issued_at":"2023-06-03T23:43:21.173248074Z"}`

var errFakeResponseNotFound = errors.New("fake response not found")

// TestGetDigestFromDocker fakes network responses, so network connection to docker.io is not needed.
func TestGetDigestFromDocker(t *testing.T) {
	// More detailed logs
	// logrus.SetLevel(logrus.DebugLevel)
	// logrus.SetFormatter(&logrus.TextFormatter{ForceColors: true})

	_, sys := setupTestCase(t)
	sys.SetTestClientRoundTripper(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		resp := &http.Response{
			Request: r,
		}
		url := r.URL.String()
		if r.Method == http.MethodGet && url == "https://registry-1.docker.io/v2/" {
			body := []byte(`{"errors":[{"code":"UNAUTHORIZED","message":"authentication required","detail":null}]}`)
			resp.Header = http.Header{}
			resp.Header.Add("Content-Type", "application/json")
			resp.Header.Add("Docker-Distribution-Api-Version", "registry/2.0")
			resp.Header.Add("Www-Authenticate", `Bearer realm="https://auth.docker.io/token",service="registry.docker.io"`)
			resp.Header.Add("Content-Length", fmt.Sprintf("%d", len(body)))
			resp.Header.Add("Strict-Transport-Security", "max-age=31536000")
			resp.Body = io.NopCloser(bytes.NewReader(body))
			resp.StatusCode = http.StatusUnauthorized
			resp.Status = http.StatusText(resp.StatusCode)
		} else if r.Method == http.MethodGet && regexp.MustCompile("https://auth.docker.io/token?.*scope=repository%3Alibrary%2Fbusybox%3Apull&service=registry.docker.io").MatchString(url) {
			resp.Header = http.Header{}
			resp.Header.Add("Content-Type", "application/json; charset=utf-8")
			resp.Header.Add("X-Trace-Id", "c00a2a131c306bbe24032054e4b958fb")
			resp.Header.Add("Strict-Transport-Security", "max-age=31536000")
			resp.Body = io.NopCloser(bytes.NewReader([]byte(tokenResp)))
			resp.StatusCode = http.StatusOK
			resp.Status = http.StatusText(resp.StatusCode)
		} else if r.Method == http.MethodHead && url == "https://registry-1.docker.io/v2/library/busybox/manifests/1.34.1" {
			resp.Header = http.Header{}
			resp.Header.Add("Content-Length", "2295")
			resp.Header.Add("Docker-Content-Digest", "sha256:05a79c7279f71f86a2a0d05eb72fcb56ea36139150f0a75cd87e80a4272e4e39")
			resp.Header.Add("Etag", `"sha256:05a79c7279f71f86a2a0d05eb72fcb56ea36139150f0a75cd87e80a4272e4e39"`)
			resp.Header.Add("Ratelimit-Remaining", "0;w=21600")
			resp.Header.Add("Docker-Ratelimit-Source", "131.228.2.25")
			resp.Header.Add("Content-Type", internalManifest.DockerV2ListMediaType)
			resp.Header.Add("Docker-Distribution-Api-Version", "registry/2.0")
			resp.Header.Add("Strict-Transport-Security", "max-age=31536000")
			resp.Header.Add("Ratelimit-Limit", "100;w=21600")
			resp.ContentLength = 2295
			resp.StatusCode = http.StatusOK
			resp.Status = http.StatusText(resp.StatusCode)
		} else {
			return nil, fmt.Errorf("%w: %s %s", errFakeResponseNotFound, r.Method, url)
		}

		return resp, nil
	}))

	imageRef, err := ParseReference("//docker.io/library/busybox:1.34.1")
	require.NoError(t, err)
	digest, err := GetDigest(context.Background(), sys, imageRef)
	assert.NoError(t, err)
	assert.Equal(t, "sha256:05a79c7279f71f86a2a0d05eb72fcb56ea36139150f0a75cd87e80a4272e4e39", string(digest))
}

func setupTestCase(t *testing.T) (string, *types.SystemContext) {
	tempHome := t.TempDir()
	rootDirPath := filepath.Join(tempHome, "root")
	err := os.MkdirAll(rootDirPath, 0700)
	require.NoError(t, err)
	userRegistriesDirPath := filepath.Join(tempHome, filepath.FromSlash(".config/containers/registries.d"))
	err = os.MkdirAll(userRegistriesDirPath, 0700)
	require.NoError(t, err)
	bigFilesDirPath := filepath.Join(rootDirPath, "containers", "bigcache")
	err = os.MkdirAll(bigFilesDirPath, 0700)
	require.NoError(t, err)

	return tempHome, &types.SystemContext{
		RootForImplicitAbsolutePaths:      rootDirPath,
		RegistriesDirPath:                 userRegistriesDirPath,
		BlobInfoCacheDir:                  filepath.Join(rootDirPath, "containers", "cache"),
		BigFilesTemporaryDir:              bigFilesDirPath,
		OCIInsecureSkipTLSVerify:          true,
		DockerInsecureSkipTLSVerify:       types.OptionalBoolTrue,
		DockerDaemonInsecureSkipTLSVerify: true,
	}
}

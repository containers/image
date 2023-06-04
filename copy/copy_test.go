package copy

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	internalManifest "github.com/containers/image/v5/internal/manifest"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	digest "github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

var errFakeResponseNotFound = errors.New("fake response not found")

// TestCopyDockerToTarball fakes network responses, so network connection to docker.io is not needed.
func TestCopyDockerToTarball(t *testing.T) {
	// More detailed logs (do not push to git)
	// logrus.SetLevel(logrus.DebugLevel)
	// logrus.SetFormatter(&logrus.TextFormatter{ForceColors: true})

	tempHome, sys := setupTestCase(t)
	policy := &signature.Policy{
		Default: []signature.PolicyRequirement{
			signature.NewPRInsecureAcceptAnything(),
		},
		Transports: map[string]signature.PolicyTransportScopes{
			"docker": map[string]signature.PolicyRequirements{
				"": []signature.PolicyRequirement{
					signature.NewPRInsecureAcceptAnything(),
				},
			},
		},
	}

	policyContext, err := signature.NewPolicyContext(policy)
	require.NoError(t, err)

	doFake := true
	if doFake {
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
			} else if r.Method == http.MethodGet && regexp.MustCompile("https://auth.docker.io/token?.*scope=repository%3Alibrary%2Fhello-world%3Apull&service=registry.docker.io").MatchString(url) {
				body := []byte(tokenResp)
				resp.Header = http.Header{}
				resp.Header.Add("Content-Type", "application/json; charset=utf-8")
				resp.Header.Add("X-Trace-Id", "c00a2a131c306bbe24032054e4b958fb")
				resp.Header.Add("Strict-Transport-Security", "max-age=31536000")
				resp.Body = io.NopCloser(bytes.NewReader(body))
				resp.StatusCode = http.StatusOK
				resp.Status = http.StatusText(resp.StatusCode)
			} else if r.Method == http.MethodGet && url == "https://registry-1.docker.io/v2/library/hello-world/manifests/latest" {
				body := []byte(helloworldManifestV2)
				resp.Header = http.Header{}
				resp.Header.Add("Content-Length", fmt.Sprintf("%d", len(body)))
				resp.Header.Add("Docker-Content-Digest", "sha256:fc6cf906cbfa013e80938cdf0bb199fbdbb86d6e3e013783e5a766f50f5dbce0")
				resp.Header.Add("Etag", `"sha256:fc6cf906cbfa013e80938cdf0bb199fbdbb86d6e3e013783e5a766f50f5dbce0"`)
				resp.Header.Add("Ratelimit-Remaining", "0;w=21600")
				resp.Header.Add("Docker-Ratelimit-Source", "131.228.2.25")
				resp.Header.Add("Content-Type", internalManifest.DockerV2ListMediaType)
				resp.Header.Add("Docker-Distribution-Api-Version", "registry/2.0")
				resp.Header.Add("Strict-Transport-Security", "max-age=31536000")
				resp.Header.Add("Ratelimit-Limit", "100;w=21600")
				resp.Body = io.NopCloser(bytes.NewReader([]byte(body)))
				resp.ContentLength = int64(len(body))
				resp.StatusCode = http.StatusOK
				resp.Status = http.StatusText(resp.StatusCode)
			} else if r.Method == http.MethodGet && url == "https://registry-1.docker.io/v2/library/hello-world/manifests/sha256:7e9b6e7ba2842c91cf49f3e214d04a7a496f8214356f41d81a6e6dcad11f11e3" {
				body := []byte(imageManifestAmd64V2)
				resp.Header = http.Header{}
				resp.Header.Add("Content-Length", fmt.Sprintf("%d", len(body)))
				resp.Header.Add("Content-Type", internalManifest.DockerV2Schema2MediaType)
				resp.Header.Add("Docker-Content-Digest", "sha256:7e9b6e7ba2842c91cf49f3e214d04a7a496f8214356f41d81a6e6dcad11f11e3")
				resp.Header.Add("Docker-Distribution-Api-Version", "registry/2.0")
				resp.Header.Add("Docker-Ratelimit-Source", "131.228.2.25")
				resp.Header.Add("Etag", `"sha256:7e9b6e7ba2842c91cf49f3e214d04a7a496f8214356f41d81a6e6dcad11f11e3"`)
				resp.Header.Add("Ratelimit-Limit", "100;w=21600")
				resp.Header.Add("Ratelimit-Remaining", "85;w=21600")
				resp.Header.Add("Strict-Transport-Security", "max-age=31536000")
				resp.Body = io.NopCloser(bytes.NewReader([]byte(body)))
				resp.ContentLength = int64(len(body))
				resp.StatusCode = http.StatusOK
				resp.Status = http.StatusText(resp.StatusCode)
			} else if r.Method == http.MethodGet && url == "https://registry-1.docker.io/v2/library/hello-world/blobs/sha256:719385e32844401d57ecfd3eacab360bf551a1491c05b85806ed8f1b08d792f6" {
				resp.Header = http.Header{}
				resp.Header.Add("Content-Type", "application/octet-stream")
				resp.Header.Add("Docker-Distribution-Api-Version", "registry/2.0")
				resp.Header.Add("Location", "https://production.cloudflare.docker.com/registry-v2/docker/registry/v2/blobs/sha256/71/719385e32844401d57ecfd3eacab360bf551a1491c05b85806ed8f1b08d792f6/data?verify=1685896700-P3OxAK20OWkR0Im6UBQH1U9Bi1c%3D")
				resp.Header.Add("Content-Length", "0")
				resp.Header.Add("Strict-Transport-Security", "max-age=31536000")
				resp.StatusCode = http.StatusTemporaryRedirect
				resp.Status = http.StatusText(resp.StatusCode)
			} else if r.Method == http.MethodGet && url == "https://production.cloudflare.docker.com/registry-v2/docker/registry/v2/blobs/sha256/71/719385e32844401d57ecfd3eacab360bf551a1491c05b85806ed8f1b08d792f6/data?verify=1685896700-P3OxAK20OWkR0Im6UBQH1U9Bi1c%3D" {
				body := []byte(imageLayerGzip)
				resp.Header = http.Header{}
				resp.Header.Add("Content-Length", fmt.Sprintf("%d", len(body)))
				resp.Header.Add("Content-Type", "application/octet-stream")
				resp.Header.Add("Etag", `"de1f8307434714775456a35bb0a2ba68"`)
				resp.Header.Add("Accept-Ranges", "bytes")
				resp.Header.Add("Server", "cloudflare")
				resp.Header.Add("Vary", "Accept-Encoding")
				resp.Body = io.NopCloser(bytes.NewReader(body))
				resp.ContentLength = int64(len(body))
				resp.StatusCode = http.StatusOK
				resp.Status = http.StatusText(resp.StatusCode)
			} else if r.Method == http.MethodGet && url == "https://registry-1.docker.io/v2/library/hello-world/blobs/sha256:9c7a54a9a43cca047013b82af109fe963fde787f63f9e016fdc3384500c2823d" {
				resp.Header = http.Header{}
				resp.Header.Add("Content-Type", "application/octet-stream")
				resp.Header.Add("Docker-Distribution-Api-Version", "registry/2.0")
				resp.Header.Add("Location", "https://production.cloudflare.docker.com/registry-v2/docker/registry/v2/blobs/sha256/9c/9c7a54a9a43cca047013b82af109fe963fde787f63f9e016fdc3384500c2823d/data?verify=1685897019-jQETp1M%2F9n0Jhpwb%2FP4dbBGGOcE%3D")
				resp.Header.Add("Content-Length", "0")
				resp.Header.Add("Strict-Transport-Security", "max-age=31536000")
				resp.StatusCode = http.StatusTemporaryRedirect
				resp.Status = http.StatusText(resp.StatusCode)
			} else if r.Method == http.MethodGet && url == "https://production.cloudflare.docker.com/registry-v2/docker/registry/v2/blobs/sha256/9c/9c7a54a9a43cca047013b82af109fe963fde787f63f9e016fdc3384500c2823d/data?verify=1685897019-jQETp1M%2F9n0Jhpwb%2FP4dbBGGOcE%3D" {
				body := []byte(imageConfigAmd64)
				resp.Header = http.Header{}
				resp.Header.Add("Content-Length", fmt.Sprintf("%d", len(body)))
				resp.Header.Add("Content-Type", "application/octet-stream")
				resp.Header.Add("Etag", `"9ea01b84bb190b3952c62c55bd32ac72"`)
				resp.Header.Add("Accept-Ranges", "bytes")
				resp.Header.Add("Server", "cloudflare")
				resp.Header.Add("Vary", "Accept-Encoding")
				resp.Body = io.NopCloser(bytes.NewReader(body))
				resp.ContentLength = int64(len(body))
				resp.StatusCode = http.StatusOK
				resp.Status = http.StatusText(resp.StatusCode)
			} else {
				return nil, fmt.Errorf("%w: %s %s", errFakeResponseNotFound, r.Method, url)
			}

			return resp, nil
		}))
	}

	srcRef, err := alltransports.ParseImageName("docker://docker.io/library/hello-world:latest")
	require.NoError(t, err)
	dstFile, err := url.JoinPath("docker-archive:", tempHome, "hello-world_latest.tar")
	require.NoError(t, err)
	dstRef, err := alltransports.ParseImageName(dstFile)
	require.NoError(t, err)

	copyOptions := &Options{
		RemoveSignatures:     false,
		ReportWriter:         os.Stdout,
		ImageListSelection:   CopySystemImage,
		SourceCtx:            sys,
		DestinationCtx:       sys,
		PreserveDigests:      true,
		MaxParallelDownloads: 1,
		// Instances is linux/amd64
		Instances: []digest.Digest{digest.Digest("sha256:7e9b6e7ba2842c91cf49f3e214d04a7a496f8214356f41d81a6e6dcad11f11e3")},
	}

	copiedManifest, err := Image(context.Background(), policyContext, dstRef, srcRef, copyOptions)
	assert.NoError(t, err)
	assert.Equal(t, []byte(imageManifestAmd64V2), copiedManifest)
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

const (
	tokenResp = `{"token":"eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCIsIng1YyI6WyJNSUlDK1RDQ0FwK2dBd0lCQWdJQkFEQUtCZ2dxaGtqT1BRUURBakJHTVVRd1FnWURWUVFERXp0U1RVbEdPbEZNUmpRNlEwZFFNenBSTWtWYU9sRklSRUk2VkVkRlZUcFZTRlZNT2taTVZqUTZSMGRXV2pwQk5WUkhPbFJMTkZNNlVVeElTVEFlRncweU16QXhNRFl3TkRJM05EUmFGdzB5TkRBeE1qWXdOREkzTkRSYU1FWXhSREJDQmdOVkJBTVRPME5EVlVZNlNqVkhOanBGUTFORU9rTldSRWM2VkRkTU1qcEtXa1pST2xOTk0wUTZXRmxQTkRwV04wTkhPa2RHVjBJNldsbzFOam8wVlVSRE1JSUJJakFOQmdrcWhraUc5dzBCQVFFRkFBT0NBUThBTUlJQkNnS0NBUUVBek4wYjBqN1V5L2FzallYV2gyZzNxbzZKaE9rQWpYV0FVQmNzSHU2aFlaUkZMOXZlODEzVEI0Y2w4UWt4Q0k0Y1VnR0duR1dYVnhIMnU1dkV0eFNPcVdCcnhTTnJoU01qL1ZPKzYvaVkrOG1GRmEwR2J5czF3VDVjNlY5cWROaERiVGNwQXVYSjFSNGJLdSt1VGpVS0VIYXlqSFI5TFBEeUdnUC9ubUFadk5PWEdtclNTSkZJNnhFNmY3QS8rOVptcWgyVlRaQlc0cXduSnF0cnNJM2NveDNQczMwS2MrYUh3V3VZdk5RdFNBdytqVXhDVVFoRWZGa0lKSzh6OVdsL1FjdE9EcEdUeXNtVHBjNzZaVEdKWWtnaGhGTFJEMmJQTlFEOEU1ZWdKa2RQOXhpaW5sVGx3MjBxWlhVRmlqdWFBcndOR0xJbUJEWE0wWlI1YzVtU3Z3SURBUUFCbzRHeU1JR3ZNQTRHQTFVZER3RUIvd1FFQXdJSGdEQVBCZ05WSFNVRUNEQUdCZ1JWSFNVQU1FUUdBMVVkRGdROUJEdERRMVZHT2tvMVJ6WTZSVU5UUkRwRFZrUkhPbFEzVERJNlNscEdVVHBUVFRORU9saFpUelE2VmpkRFJ6cEhSbGRDT2xwYU5UWTZORlZFUXpCR0JnTlZIU01FUHpBOWdEdFNUVWxHT2xGTVJqUTZRMGRRTXpwUk1rVmFPbEZJUkVJNlZFZEZWVHBWU0ZWTU9rWk1WalE2UjBkV1dqcEJOVlJIT2xSTE5GTTZVVXhJU1RBS0JnZ3Foa2pPUFFRREFnTklBREJGQWlFQW1RNHhsQXZXVlArTy9hNlhDU05pYUFYRU1Bb1RQVFRYRWJYMks2RVU4ZTBDSUg0QTAwSVhtUndjdGtEOHlYNzdkTVoyK0pEY1FGdDFxRktMZFR5SnVzT1UiXX0.eyJhY2Nlc3MiOlt7InR5cGUiOiJyZXBvc2l0b3J5IiwibmFtZSI6ImxpYnJhcnkvaGVsbG8td29ybGQiLCJhY3Rpb25zIjpbInB1bGwiXSwicGFyYW1ldGVycyI6eyJwdWxsX2xpbWl0IjoiMTAwIiwicHVsbF9saW1pdF9pbnRlcnZhbCI6IjIxNjAwIn19XSwiYXVkIjoicmVnaXN0cnkuZG9ja2VyLmlvIiwiZXhwIjoxNjg1ODkzMDY4LCJpYXQiOjE2ODU4OTI3NjgsImlzcyI6ImF1dGguZG9ja2VyLmlvIiwianRpIjoiZGNrcl9qdGlfeGJGWnc2NnRINGJWWWYzb3lpNk9kZjMzVnlrPSIsIm5iZiI6MTY4NTg5MjQ2OCwic3ViIjoiIn0.OPq1FnKgsMfnJqRN88KS441bNtgTJH4wNsa5ZZ8H9tirBowCTznKRdHnn21Hb5yfOsnaFwxF3D2R3tWy5bR9BOZm5x6XqnZQFN1d24X_lZOknfMKA-SPTeZSG_Q8NR8PQ9vEJKhB3VthXRvop1R6oNE5uFVxksZ-bJtPgLWOUzOLL3fi_VMMfWPTLT9468AY57o42D7qWAKfBHBI96ZF2ZMK1CdjSQ14jw-CqRjXeXhHFr7FGS9vbs4nlOgioJpip9lZ3Tv2wlomymB7NgfXSREppPBAs8EW_lKVnAVx_yYqk5npRfzbJqY5mHjPSax6vtEFQymayvPAOFaHtg_wNw","access_token":"eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCIsIng1YyI6WyJNSUlDK1RDQ0FwK2dBd0lCQWdJQkFEQUtCZ2dxaGtqT1BRUURBakJHTVVRd1FnWURWUVFERXp0U1RVbEdPbEZNUmpRNlEwZFFNenBSTWtWYU9sRklSRUk2VkVkRlZUcFZTRlZNT2taTVZqUTZSMGRXV2pwQk5WUkhPbFJMTkZNNlVVeElTVEFlRncweU16QXhNRFl3TkRJM05EUmFGdzB5TkRBeE1qWXdOREkzTkRSYU1FWXhSREJDQmdOVkJBTVRPME5EVlVZNlNqVkhOanBGUTFORU9rTldSRWM2VkRkTU1qcEtXa1pST2xOTk0wUTZXRmxQTkRwV04wTkhPa2RHVjBJNldsbzFOam8wVlVSRE1JSUJJakFOQmdrcWhraUc5dzBCQVFFRkFBT0NBUThBTUlJQkNnS0NBUUVBek4wYjBqN1V5L2FzallYV2gyZzNxbzZKaE9rQWpYV0FVQmNzSHU2aFlaUkZMOXZlODEzVEI0Y2w4UWt4Q0k0Y1VnR0duR1dYVnhIMnU1dkV0eFNPcVdCcnhTTnJoU01qL1ZPKzYvaVkrOG1GRmEwR2J5czF3VDVjNlY5cWROaERiVGNwQXVYSjFSNGJLdSt1VGpVS0VIYXlqSFI5TFBEeUdnUC9ubUFadk5PWEdtclNTSkZJNnhFNmY3QS8rOVptcWgyVlRaQlc0cXduSnF0cnNJM2NveDNQczMwS2MrYUh3V3VZdk5RdFNBdytqVXhDVVFoRWZGa0lKSzh6OVdsL1FjdE9EcEdUeXNtVHBjNzZaVEdKWWtnaGhGTFJEMmJQTlFEOEU1ZWdKa2RQOXhpaW5sVGx3MjBxWlhVRmlqdWFBcndOR0xJbUJEWE0wWlI1YzVtU3Z3SURBUUFCbzRHeU1JR3ZNQTRHQTFVZER3RUIvd1FFQXdJSGdEQVBCZ05WSFNVRUNEQUdCZ1JWSFNVQU1FUUdBMVVkRGdROUJEdERRMVZHT2tvMVJ6WTZSVU5UUkRwRFZrUkhPbFEzVERJNlNscEdVVHBUVFRORU9saFpUelE2VmpkRFJ6cEhSbGRDT2xwYU5UWTZORlZFUXpCR0JnTlZIU01FUHpBOWdEdFNUVWxHT2xGTVJqUTZRMGRRTXpwUk1rVmFPbEZJUkVJNlZFZEZWVHBWU0ZWTU9rWk1WalE2UjBkV1dqcEJOVlJIT2xSTE5GTTZVVXhJU1RBS0JnZ3Foa2pPUFFRREFnTklBREJGQWlFQW1RNHhsQXZXVlArTy9hNlhDU05pYUFYRU1Bb1RQVFRYRWJYMks2RVU4ZTBDSUg0QTAwSVhtUndjdGtEOHlYNzdkTVoyK0pEY1FGdDFxRktMZFR5SnVzT1UiXX0.eyJhY2Nlc3MiOlt7InR5cGUiOiJyZXBvc2l0b3J5IiwibmFtZSI6ImxpYnJhcnkvaGVsbG8td29ybGQiLCJhY3Rpb25zIjpbInB1bGwiXSwicGFyYW1ldGVycyI6eyJwdWxsX2xpbWl0IjoiMTAwIiwicHVsbF9saW1pdF9pbnRlcnZhbCI6IjIxNjAwIn19XSwiYXVkIjoicmVnaXN0cnkuZG9ja2VyLmlvIiwiZXhwIjoxNjg1ODkzMDY4LCJpYXQiOjE2ODU4OTI3NjgsImlzcyI6ImF1dGguZG9ja2VyLmlvIiwianRpIjoiZGNrcl9qdGlfeGJGWnc2NnRINGJWWWYzb3lpNk9kZjMzVnlrPSIsIm5iZiI6MTY4NTg5MjQ2OCwic3ViIjoiIn0.OPq1FnKgsMfnJqRN88KS441bNtgTJH4wNsa5ZZ8H9tirBowCTznKRdHnn21Hb5yfOsnaFwxF3D2R3tWy5bR9BOZm5x6XqnZQFN1d24X_lZOknfMKA-SPTeZSG_Q8NR8PQ9vEJKhB3VthXRvop1R6oNE5uFVxksZ-bJtPgLWOUzOLL3fi_VMMfWPTLT9468AY57o42D7qWAKfBHBI96ZF2ZMK1CdjSQ14jw-CqRjXeXhHFr7FGS9vbs4nlOgioJpip9lZ3Tv2wlomymB7NgfXSREppPBAs8EW_lKVnAVx_yYqk5npRfzbJqY5mHjPSax6vtEFQymayvPAOFaHtg_wNw","expires_in":300,"issued_at":"2023-06-04T15:32:48.548468494Z"}`

	helloworldManifestV2 = `{"manifests":[{"digest":"sha256:7e9b6e7ba2842c91cf49f3e214d04a7a496f8214356f41d81a6e6dcad11f11e3","mediaType":"application\/vnd.docker.distribution.manifest.v2+json","platform":{"architecture":"amd64","os":"linux"},"size":525},{"digest":"sha256:084c3bdd1271adc754e2c5f6ba7046f1a2c099597dbd9643592fa8eb99981402","mediaType":"application\/vnd.docker.distribution.manifest.v2+json","platform":{"architecture":"arm","os":"linux","variant":"v5"},"size":525},{"digest":"sha256:a0a386314d69d1514d7aa63d12532b284bf37bba15ed7b4fc1a3f86605f86c63","mediaType":"application\/vnd.docker.distribution.manifest.v2+json","platform":{"architecture":"arm","os":"linux","variant":"v7"},"size":525},{"digest":"sha256:efebf0f7aee69450f99deafe11121afa720abed733943e50581a9dc7540689c8","mediaType":"application\/vnd.docker.distribution.manifest.v2+json","platform":{"architecture":"arm64","os":"linux","variant":"v8"},"size":525},{"digest":"sha256:004d23c66201b22fce069b7505756f17088de7889c83891e9bc69d749fa3690e","mediaType":"application\/vnd.docker.distribution.manifest.v2+json","platform":{"architecture":"386","os":"linux"},"size":525},{"digest":"sha256:06bca41ba617acf0b3644df05d0d9c2d2f82ccaab629c0e39792b24682970040","mediaType":"application\/vnd.docker.distribution.manifest.v2+json","platform":{"architecture":"mips64le","os":"linux"},"size":525},{"digest":"sha256:fbe0ff1e7697da39d987a975c737a7d2fa40b6e7f7f40c00b1dcc387b9ac0e85","mediaType":"application\/vnd.docker.distribution.manifest.v2+json","platform":{"architecture":"ppc64le","os":"linux"},"size":525},{"digest":"sha256:72ba79e34f1baa40cd4d9ecd684b8389d0a1e18cf6e6d5c44c19716d25f65e20","mediaType":"application\/vnd.docker.distribution.manifest.v2+json","platform":{"architecture":"riscv64","os":"linux"},"size":525},{"digest":"sha256:574efe68740d3bee2ef780036aee2e2da5cf7097ac06513f9f01f41e03365399","mediaType":"application\/vnd.docker.distribution.manifest.v2+json","platform":{"architecture":"s390x","os":"linux"},"size":525},{"digest":"sha256:85481f9fb05b83c912dfdec9cea6230f2df24e5dfde84a23def8915cec2519b5","mediaType":"application\/vnd.docker.distribution.manifest.v2+json","platform":{"architecture":"amd64","os":"windows","os.version":"10.0.20348.1726"},"size":946},{"digest":"sha256:3522a799ae11b49426f6a60b3fcf3d249f21fbd3dc1dd346d8a49fe4f028b668","mediaType":"application\/vnd.docker.distribution.manifest.v2+json","platform":{"architecture":"amd64","os":"windows","os.version":"10.0.17763.4377"},"size":946}],"mediaType":"application\/vnd.docker.distribution.manifest.list.v2+json","schemaVersion":2}`

	imageManifestAmd64V2 = `{
   "schemaVersion": 2,
   "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
   "config": {
      "mediaType": "application/vnd.docker.container.image.v1+json",
      "size": 1470,
      "digest": "sha256:9c7a54a9a43cca047013b82af109fe963fde787f63f9e016fdc3384500c2823d"
   },
   "layers": [
      {
         "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
         "size": 2457,
         "digest": "sha256:719385e32844401d57ecfd3eacab360bf551a1491c05b85806ed8f1b08d792f6"
      }
   ]
}`

	imageConfigAmd64 = `{"architecture":"amd64","config":{"Hostname":"","Domainname":"","User":"","AttachStdin":false,"AttachStdout":false,"AttachStderr":false,"Tty":false,"OpenStdin":false,"StdinOnce":false,"Env":["PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"],"Cmd":["/hello"],"Image":"sha256:62a15619037f3c4fb4e6ba9bd224cba3540e393a55dc52f6bebe212ca7b5e1a7","Volumes":null,"WorkingDir":"","Entrypoint":null,"OnBuild":null,"Labels":null},"container":"347ca68872ee924c4f9394b195dcadaf591d387a45d624225251efc6cb7a348e","container_config":{"Hostname":"347ca68872ee","Domainname":"","User":"","AttachStdin":false,"AttachStdout":false,"AttachStderr":false,"Tty":false,"OpenStdin":false,"StdinOnce":false,"Env":["PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"],"Cmd":["/bin/sh","-c","#(nop) ","CMD [\"/hello\"]"],"Image":"sha256:62a15619037f3c4fb4e6ba9bd224cba3540e393a55dc52f6bebe212ca7b5e1a7","Volumes":null,"WorkingDir":"","Entrypoint":null,"OnBuild":null,"Labels":{}},"created":"2023-05-04T17:37:03.872958712Z","docker_version":"20.10.23","history":[{"created":"2023-05-04T17:37:03.801840823Z","created_by":"/bin/sh -c #(nop) COPY file:201f8f1849e89d53be9f6aa76937f5e209d745abfd15a8552fcf2ba45ab267f9 in / "},{"created":"2023-05-04T17:37:03.872958712Z","created_by":"/bin/sh -c #(nop)  CMD [\"/hello\"]","empty_layer":true}],"os":"linux","rootfs":{"type":"layers","diff_ids":["sha256:01bb4fce3eb1b56b05adf99504dafd31907a5aadac736e36b27595c8b92f07f1"]}}`
)

var (
	//imageLayerGzip was downloaded by:
	//  TOKEN=$(curl -ks 'https://auth.docker.io/token?scope=repository%3Alibrary%2Fhello-world%3Apull&service=registry.docker.io' |jq -r '.token')
	//  curl -v -L -H "Authorization: Bearer $TOKEN" 'https://registry-1.docker.io/v2/library/hello-world/blobs/sha256:719385e32844401d57ecfd3eacab360bf551a1491c05b85806ed8f1b08d792f6' -o fixtures/hello-world_layer.gz
	//go:embed fixtures/hello-world_layer.gz
	imageLayerGzip []byte
)

package docker

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/containers/image/v5/internal/useragent"
	"github.com/containers/image/v5/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDockerCertDir(t *testing.T) {
	const nondefaultFullPath = "/this/is/not/the/default/full/path"
	const nondefaultPerHostDir = "/this/is/not/the/default/certs.d"
	const variableReference = "$HOME"
	const rootPrefix = "/root/prefix"
	const registryHostPort = "thishostdefinitelydoesnotexist:5000"

	systemPerHostResult := filepath.Join(perHostCertDirs[len(perHostCertDirs)-1].path, registryHostPort)
	for _, c := range []struct {
		sys      *types.SystemContext
		expected string
	}{
		// The common case
		{nil, systemPerHostResult},
		// There is a context, but it does not override the path.
		{&types.SystemContext{}, systemPerHostResult},
		// Full path overridden
		{&types.SystemContext{DockerCertPath: nondefaultFullPath}, nondefaultFullPath},
		// Per-host path overridden
		{
			&types.SystemContext{DockerPerHostCertDirPath: nondefaultPerHostDir},
			filepath.Join(nondefaultPerHostDir, registryHostPort),
		},
		// Both overridden
		{
			&types.SystemContext{
				DockerCertPath:           nondefaultFullPath,
				DockerPerHostCertDirPath: nondefaultPerHostDir,
			},
			nondefaultFullPath,
		},
		// Root overridden
		{
			&types.SystemContext{RootForImplicitAbsolutePaths: rootPrefix},
			filepath.Join(rootPrefix, systemPerHostResult),
		},
		// Root and path overrides present simultaneously,
		{
			&types.SystemContext{
				DockerCertPath:               nondefaultFullPath,
				RootForImplicitAbsolutePaths: rootPrefix,
			},
			nondefaultFullPath,
		},
		{
			&types.SystemContext{
				DockerPerHostCertDirPath:     nondefaultPerHostDir,
				RootForImplicitAbsolutePaths: rootPrefix,
			},
			filepath.Join(nondefaultPerHostDir, registryHostPort),
		},
		// … and everything at once
		{
			&types.SystemContext{
				DockerCertPath:               nondefaultFullPath,
				DockerPerHostCertDirPath:     nondefaultPerHostDir,
				RootForImplicitAbsolutePaths: rootPrefix,
			},
			nondefaultFullPath,
		},
		// No environment expansion happens in the overridden paths
		{&types.SystemContext{DockerCertPath: variableReference}, variableReference},
		{
			&types.SystemContext{DockerPerHostCertDirPath: variableReference},
			filepath.Join(variableReference, registryHostPort),
		},
	} {
		path, err := dockerCertDir(c.sys, registryHostPort)
		require.Equal(t, nil, err)
		assert.Equal(t, c.expected, path)
	}
}

// testTokenHTTPResponse creates just enough of a *http.Response to work with newBearerTokenFromHTTPResponseBody.
func testTokenHTTPResponse(t *testing.T, body string) *http.Response {
	requestURL, err := url.Parse("https://example.com/token")
	require.NoError(t, err)
	return &http.Response{
		Body: io.NopCloser(bytes.NewReader([]byte(body))),
		Request: &http.Request{
			Method: "",
			URL:    requestURL,
		},
	}
}

func TestBearerTokenReadFromHTTPResponseBody(t *testing.T) {
	for _, c := range []struct {
		input    string
		expected *bearerToken // or nil if a failure is expected
	}{
		{ // Invalid JSON
			input:    "IAmNotJson",
			expected: nil,
		},
		{ // "token"
			input:    `{"token":"IAmAToken","expires_in":100,"issued_at":"2018-01-01T10:00:02+00:00"}`,
			expected: &bearerToken{token: "IAmAToken", expirationTime: time.Unix(1514800802+100, 0)},
		},
		{ // "access_token"
			input:    `{"access_token":"IAmAToken","expires_in":100,"issued_at":"2018-01-01T10:00:02+00:00"}`,
			expected: &bearerToken{token: "IAmAToken", expirationTime: time.Unix(1514800802+100, 0)},
		},
		{ // Small expiry
			input:    `{"token":"IAmAToken","expires_in":1,"issued_at":"2018-01-01T10:00:02+00:00"}`,
			expected: &bearerToken{token: "IAmAToken", expirationTime: time.Unix(1514800802+60, 0)},
		},
	} {
		token := &bearerToken{}
		err := token.readFromHTTPResponseBody(testTokenHTTPResponse(t, c.input))
		if c.expected == nil {
			assert.Error(t, err, c.input)
		} else {
			require.NoError(t, err, c.input)
			assert.Equal(t, c.expected.token, token.token, c.input)
			assert.True(t, c.expected.expirationTime.Equal(token.expirationTime),
				"expected [%s] to equal [%s], it did not", token.expirationTime, c.expected.expirationTime)
		}
	}
}

func TestBearerTokenReadFromHTTPResponseBodyIssuedAtZero(t *testing.T) {
	zeroTime := time.Time{}.Format(time.RFC3339)
	now := time.Now()
	tokenBlob := fmt.Sprintf(`{"token":"IAmAToken","expires_in":100,"issued_at":"%s"}`, zeroTime)
	token := &bearerToken{}
	err := token.readFromHTTPResponseBody(testTokenHTTPResponse(t, tokenBlob))
	require.NoError(t, err)
	expectedExpiration := now.Add(time.Duration(100) * time.Second)
	require.False(t, token.expirationTime.Before(expectedExpiration),
		"expected [%s] not to be before [%s]", token.expirationTime, expectedExpiration)
}

func TestUserAgent(t *testing.T) {
	const sentinelUA = "sentinel/1.0"

	var expectedUA string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("User-Agent")
		assert.Equal(t, expectedUA, got)
		w.WriteHeader(http.StatusOK)
	}))
	defer s.Close()

	for _, tc := range []struct {
		sys      *types.SystemContext
		expected string
	}{
		// Can't both test nil and set DockerInsecureSkipTLSVerify :(
		// {nil, defaultUA},
		{&types.SystemContext{}, useragent.DefaultUserAgent},
		{&types.SystemContext{DockerRegistryUserAgent: sentinelUA}, sentinelUA},
	} {
		// For this test against localhost, we don't care.
		tc.sys.DockerInsecureSkipTLSVerify = types.OptionalBoolTrue

		registry := strings.TrimPrefix(s.URL, "http://")

		expectedUA = tc.expected
		if err := CheckAuth(context.Background(), tc.sys, "", "", registry); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

var registrySuseComResp = http.Response{
	Status:     "401 Unauthorized",
	StatusCode: http.StatusUnauthorized,
	Proto:      "HTTP/1.1",
	ProtoMajor: 1,
	ProtoMinor: 1,
	Header: map[string][]string{
		"Content-Length":                  {"145"},
		"Content-Type":                    {"application/json"},
		"Date":                            {"Fri, 26 Aug 2022 08:03:13 GMT"},
		"Docker-Distribution-Api-Version": {"registry/2.0"},
		// "Www-Authenticate":                {`Bearer realm="https://registry.suse.com/auth",service="SUSE Linux Docker Registry",scope="registry:catalog:*",error="insufficient_scope"`},
		"X-Content-Type-Options": {"nosniff"},
	},
	Request: nil,
}

func TestNeedsRetryOnInsuficientScope(t *testing.T) {
	resp := registrySuseComResp
	resp.Header["Www-Authenticate"] = []string{
		`Bearer realm="https://registry.suse.com/auth",service="SUSE Linux Docker Registry",scope="registry:catalog:*",error="insufficient_scope"`,
	}
	expectedScope := authScope{
		resourceType: "registry",
		remoteName:   "catalog",
		actions:      "*",
	}

	needsRetry, scope := needsRetryWithUpdatedScope(&resp)

	if !needsRetry {
		t.Fatal("Expected needing to retry")
	}

	if expectedScope != *scope {
		t.Fatalf("Got an invalid scope, expected '%q' but got '%q'", expectedScope, *scope)
	}
}

func TestNeedsRetryNoRetryWhenNoAuthHeader(t *testing.T) {
	resp := registrySuseComResp
	delete(resp.Header, "Www-Authenticate")

	needsRetry, _ := needsRetryWithUpdatedScope(&resp)

	if needsRetry {
		t.Fatal("Expected no need to retry, as no Authentication headers are present")
	}
}

func TestNeedsRetryNoRetryWhenNoBearerAuthHeader(t *testing.T) {
	resp := registrySuseComResp
	resp.Header["Www-Authenticate"] = []string{
		`OAuth2 realm="https://registry.suse.com/auth",service="SUSE Linux Docker Registry",scope="registry:catalog:*"`,
	}

	needsRetry, _ := needsRetryWithUpdatedScope(&resp)

	if needsRetry {
		t.Fatal("Expected no need to retry, as no bearer authentication header is present")
	}
}

func TestNeedsRetryNoRetryWhenNoErrorInBearer(t *testing.T) {
	resp := registrySuseComResp
	resp.Header["Www-Authenticate"] = []string{
		`Bearer realm="https://registry.suse.com/auth",service="SUSE Linux Docker Registry",scope="registry:catalog:*"`,
	}

	needsRetry, _ := needsRetryWithUpdatedScope(&resp)

	if needsRetry {
		t.Fatal("Expected no need to retry, as no insufficient error is present in the authentication header")
	}
}

func TestNeedsRetryNoRetryWhenInvalidErrorInBearer(t *testing.T) {
	resp := registrySuseComResp
	resp.Header["Www-Authenticate"] = []string{
		`Bearer realm="https://registry.suse.com/auth",service="SUSE Linux Docker Registry",scope="registry:catalog:*,error="random_error"`,
	}

	needsRetry, _ := needsRetryWithUpdatedScope(&resp)

	if needsRetry {
		t.Fatal("Expected no need to retry, as no insufficient_error is present in the authentication header")
	}
}

func TestNeedsRetryNoRetryWhenInvalidScope(t *testing.T) {
	resp := registrySuseComResp
	resp.Header["Www-Authenticate"] = []string{
		`Bearer realm="https://registry.suse.com/auth",service="SUSE Linux Docker Registry",scope="foo:bar",error="insufficient_scope"`,
	}

	needsRetry, _ := needsRetryWithUpdatedScope(&resp)

	if needsRetry {
		t.Fatal("Expected no need to retry, as no insufficient_error is present in the authentication header")
	}
}

func TestNeedsNoRetry(t *testing.T) {
	resp := http.Response{
		Status:     "200 OK",
		StatusCode: http.StatusOK,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header: map[string][]string{"Apptime": {"D=49722"},
			"Content-Length":                  {"1683"},
			"Content-Type":                    {"application/json; charset=utf-8"},
			"Date":                            {"Fri, 26 Aug 2022 09:00:21 GMT"},
			"Docker-Distribution-Api-Version": {"registry/2.0"},
			"Link":                            {`</v2/_catalog?last=f35%2Fs2i-base&n=100>; rel="next"`},
			"Referrer-Policy":                 {"same-origin"},
			"Server":                          {"Apache"},
			"Strict-Transport-Security":       {"max-age=31536000; includeSubDomains; preload"},
			"Vary":                            {"Accept"},
			"X-Content-Type-Options":          {"nosniff"},
			"X-Fedora-Proxyserver":            {"proxy10.iad2.fedoraproject.org"},
			"X-Fedora-Requestid":              {"YwiLpHEhLsbSTugJblBF8QAAAEI"},
			"X-Frame-Options":                 {"SAMEORIGIN"},
			"X-Xss-Protection":                {"1; mode=block"},
		},
	}

	needsRetry, _ := needsRetryWithUpdatedScope(&resp)
	if needsRetry {
		t.Fatal("Got the need to retry, but none should be required")
	}
}

func TestParseRegistryWarningHeader(t *testing.T) {
	for _, c := range []struct{ header, expected string }{
		{"completely invalid", ""},
		{`299 - "trivial"`, "trivial"},
		{`100 - "not-299"`, ""},
		{`299 localhost "warn-agent set"`, ""},
		{`299 - "no-terminating-quote`, ""},
		{"299 - \"\x01 control\"", ""},
		{"299 - \"\\\x01 escaped control\"", ""},
		{"299 - \"e\\scaped\"", "escaped"},
		{"299 - \"non-UTF8 \xA1\xA2\"", "non-UTF8 \xA1\xA2"},
		{"299 - \"non-UTF8 escaped \\\xA1\\\xA2\"", "non-UTF8 escaped \xA1\xA2"},
		{"299 - \"UTF8 žluťoučký\"", "UTF8 žluťoučký"},
		{"299 - \"UTF8 \\\xC5\\\xBEluťoučký\"", "UTF8 žluťoučký"},
		{`299 - "unterminated`, ""},
		{`299 - "warning" "some-date"`, ""},
	} {
		res := parseRegistryWarningHeader(c.header)
		assert.Equal(t, c.expected, res, c.header)
	}
}

func TestIsManifestUnknownError(t *testing.T) {
	// Mostly a smoke test; we can add more registries here if they need special handling.

	for _, c := range []struct{ name, response string }{
		{
			name: "docker.io when a tag in an _existing repo_ is not found",
			response: "HTTP/1.1 404 Not Found\r\n" +
				"Connection: close\r\n" +
				"Content-Length: 109\r\n" +
				"Content-Type: application/json\r\n" +
				"Date: Thu, 12 Aug 2021 20:51:32 GMT\r\n" +
				"Docker-Distribution-Api-Version: registry/2.0\r\n" +
				"Ratelimit-Limit: 100;w=21600\r\n" +
				"Ratelimit-Remaining: 100;w=21600\r\n" +
				"Strict-Transport-Security: max-age=31536000\r\n" +
				"\r\n" +
				"{\"errors\":[{\"code\":\"MANIFEST_UNKNOWN\",\"message\":\"manifest unknown\",\"detail\":{\"Tag\":\"this-does-not-exist\"}}]}\n",
		},
		{
			name: "registry.redhat.io/v2/this-does-not-exist/manifests/latest",
			response: "HTTP/1.1 404 Not Found\r\n" +
				"Connection: close\r\n" +
				"Content-Length: 53\r\n" +
				"Cache-Control: max-age=0, no-cache, no-store\r\n" +
				"Content-Type: application/json\r\n" +
				"Date: Thu, 13 Oct 2022 18:15:15 GMT\r\n" +
				"Expires: Thu, 13 Oct 2022 18:15:15 GMT\r\n" +
				"Pragma: no-cache\r\n" +
				"Server: Apache\r\n" +
				"Strict-Transport-Security: max-age=63072000; includeSubdomains; preload\r\n" +
				"X-Hostname: crane-tbr06.cran-001.prod.iad2.dc.redhat.com\r\n" +
				"\r\n" +
				"{\"errors\": [{\"code\": \"404\", \"message\": \"Not Found\"}]}\r\n",
		},
		{
			name: "registry.redhat.io/v2/rhosp15-rhel8/openstack-cron/manifests/sha256-8df5e60c42668706ac108b59c559b9187fa2de7e4e262e2967e3e9da35d5a8d7.sig",
			response: "HTTP/1.1 404 Not Found\r\n" +
				"Connection: close\r\n" +
				"Content-Length: 10\r\n" +
				"Accept-Ranges: bytes\r\n" +
				"Date: Thu, 13 Oct 2022 18:13:53 GMT\r\n" +
				"Server: AkamaiNetStorage\r\n" +
				"X-Docker-Size: -1\r\n" +
				"\r\n" +
				"Not found\r\n",
		},
		{
			name: "Harbor v2.10.2",
			response: "HTTP/1.1 404 Not Found\r\n" +
				"Content-Length: 153\r\n" +
				"Connection: keep-alive\r\n" +
				"Content-Type: application/json; charset=utf-8\r\n" +
				"Date: Wed, 08 May 2024 08:14:59 GMT\r\n" +
				"Server: nginx\r\n" +
				"Set-Cookie: sid=f617c257877837614ada2561513d6827; Path=/; HttpOnly\r\n" +
				"X-Request-Id: 1b151fb1-c943-4190-a9ce-5156ed5e3200\r\n" +
				"\r\n" +
				"{\"errors\":[{\"code\":\"NOT_FOUND\",\"message\":\"artifact test/alpine:sha256-443205b0cfcc78444321d56a2fe273f06e27b2c72b5058f8d7e975997d45b015.sig not found\"}]}\n",
		},
	} {
		resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader([]byte(c.response))), nil)
		require.NoError(t, err, c.name)
		defer resp.Body.Close()
		err = fmt.Errorf("wrapped: %w", registryHTTPResponseToError(resp))

		res := isManifestUnknownError(err)
		assert.True(t, res, "%s: %#v", c.name, err)
	}
}

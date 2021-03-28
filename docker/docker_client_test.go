package docker

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestNewBearerTokenFromJsonBlob(t *testing.T) {
	expected := &bearerToken{Token: "IAmAToken", ExpiresIn: 100, IssuedAt: time.Unix(1514800802, 0)}
	tokenBlob := []byte(`{"token":"IAmAToken","expires_in":100,"issued_at":"2018-01-01T10:00:02+00:00"}`)
	token, err := newBearerTokenFromJSONBlob(tokenBlob)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertBearerTokensEqual(t, expected, token)
}

func TestNewBearerAccessTokenFromJsonBlob(t *testing.T) {
	expected := &bearerToken{Token: "IAmAToken", ExpiresIn: 100, IssuedAt: time.Unix(1514800802, 0)}
	tokenBlob := []byte(`{"access_token":"IAmAToken","expires_in":100,"issued_at":"2018-01-01T10:00:02+00:00"}`)
	token, err := newBearerTokenFromJSONBlob(tokenBlob)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertBearerTokensEqual(t, expected, token)
}

func TestNewBearerTokenFromInvalidJsonBlob(t *testing.T) {
	tokenBlob := []byte("IAmNotJson")
	_, err := newBearerTokenFromJSONBlob(tokenBlob)
	if err == nil {
		t.Fatalf("unexpected an error unmarshaling JSON")
	}
}

func TestNewBearerTokenSmallExpiryFromJsonBlob(t *testing.T) {
	expected := &bearerToken{Token: "IAmAToken", ExpiresIn: 60, IssuedAt: time.Unix(1514800802, 0)}
	tokenBlob := []byte(`{"token":"IAmAToken","expires_in":1,"issued_at":"2018-01-01T10:00:02+00:00"}`)
	token, err := newBearerTokenFromJSONBlob(tokenBlob)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertBearerTokensEqual(t, expected, token)
}

func TestNewBearerTokenIssuedAtZeroFromJsonBlob(t *testing.T) {
	zeroTime := time.Time{}.Format(time.RFC3339)
	now := time.Now()
	tokenBlob := []byte(fmt.Sprintf(`{"token":"IAmAToken","expires_in":100,"issued_at":"%s"}`, zeroTime))
	token, err := newBearerTokenFromJSONBlob(tokenBlob)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if token.IssuedAt.Before(now) {
		t.Fatalf("expected [%s] not to be before [%s]", token.IssuedAt, now)
	}

}

func assertBearerTokensEqual(t *testing.T, expected, subject *bearerToken) {
	if expected.Token != subject.Token {
		t.Fatalf("expected [%s] to equal [%s], it did not", subject.Token, expected.Token)
	}
	if expected.ExpiresIn != subject.ExpiresIn {
		t.Fatalf("expected [%d] to equal [%d], it did not", subject.ExpiresIn, expected.ExpiresIn)
	}
	if !expected.IssuedAt.Equal(subject.IssuedAt) {
		t.Fatalf("expected [%s] to equal [%s], it did not", subject.IssuedAt, expected.IssuedAt)
	}
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
		{&types.SystemContext{}, defaultUserAgent},
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

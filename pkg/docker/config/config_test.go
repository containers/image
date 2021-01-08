package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/containers/image/v5/types"
	"github.com/containers/storage/pkg/homedir"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPathToAuth(t *testing.T) {
	uid := fmt.Sprintf("%d", os.Getuid())

	tmpDir, err := ioutil.TempDir("", "TestGetPathToAuth")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Environment is per-process, so this looks very unsafe; actually it seems fine because tests are not
	// run in parallel unless they opt in by calling t.Parallel().  So don’t do that.
	oldXRD, hasXRD := os.LookupEnv("XDG_RUNTIME_DIR")
	defer func() {
		if hasXRD {
			os.Setenv("XDG_RUNTIME_DIR", oldXRD)
		} else {
			os.Unsetenv("XDG_RUNTIME_DIR")
		}
	}()

	for _, c := range []struct {
		sys          *types.SystemContext
		xrd          string
		expected     string
		legacyFormat bool
	}{
		// Default paths
		{&types.SystemContext{}, "", "/run/containers/" + uid + "/auth.json", false},
		{nil, "", "/run/containers/" + uid + "/auth.json", false},
		// SystemContext overrides
		{&types.SystemContext{AuthFilePath: "/absolute/path"}, "", "/absolute/path", false},
		{&types.SystemContext{LegacyFormatAuthFilePath: "/absolute/path"}, "", "/absolute/path", true},
		{&types.SystemContext{RootForImplicitAbsolutePaths: "/prefix"}, "", "/prefix/run/containers/" + uid + "/auth.json", false},
		// XDG_RUNTIME_DIR defined
		{nil, tmpDir, tmpDir + "/containers/auth.json", false},
		{nil, tmpDir + "/thisdoesnotexist", "", false},
	} {
		if c.xrd != "" {
			os.Setenv("XDG_RUNTIME_DIR", c.xrd)
		} else {
			os.Unsetenv("XDG_RUNTIME_DIR")
		}
		res, lf, err := getPathToAuth(c.sys)
		if c.expected == "" {
			assert.Error(t, err)
		} else {
			require.NoError(t, err)
			assert.Equal(t, c.expected, res)
			assert.Equal(t, c.legacyFormat, lf)
		}
	}
}

func TestGetAuth(t *testing.T) {
	origXDG := os.Getenv("XDG_RUNTIME_DIR")
	tmpXDGRuntimeDir, err := ioutil.TempDir("", "test_docker_client_get_auth")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("using temporary XDG_RUNTIME_DIR directory: %q", tmpXDGRuntimeDir)
	// override XDG_RUNTIME_DIR
	os.Setenv("XDG_RUNTIME_DIR", tmpXDGRuntimeDir)
	defer func() {
		err := os.RemoveAll(tmpXDGRuntimeDir)
		if err != nil {
			t.Logf("failed to cleanup temporary home directory %q: %v", tmpXDGRuntimeDir, err)
		}
		os.Setenv("XDG_RUNTIME_DIR", origXDG)
	}()

	origHomeDir := homedir.Get()
	tmpHomeDir, err := ioutil.TempDir("", "test_docker_client_get_auth")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("using temporary home directory: %q", tmpHomeDir)
	//override homedir
	os.Setenv(homedir.Key(), tmpHomeDir)
	defer func() {
		err := os.RemoveAll(tmpHomeDir)
		if err != nil {
			t.Logf("failed to cleanup temporary home directory %q: %v", tmpHomeDir, err)
		}
		os.Setenv(homedir.Key(), origHomeDir)
	}()

	configDir1 := filepath.Join(tmpXDGRuntimeDir, "containers")
	if err := os.MkdirAll(configDir1, 0700); err != nil {
		t.Fatal(err)
	}
	configDir2 := filepath.Join(tmpHomeDir, ".docker")
	if err := os.MkdirAll(configDir2, 0700); err != nil {
		t.Fatal(err)
	}
	configPaths := [2]string{filepath.Join(configDir1, "auth.json"), filepath.Join(configDir2, "config.json")}

	for _, configPath := range configPaths {
		for _, tc := range []struct {
			name          string
			hostname      string
			path          string
			expected      types.DockerAuthConfig
			expectedError error
			sys           *types.SystemContext
		}{
			{
				name:     "no auth config",
				hostname: "index.docker.io",
			},
			{
				name: "empty hostname",
				path: filepath.Join("testdata", "example.json"),
			},
			{
				name:     "match one",
				hostname: "example.org",
				path:     filepath.Join("testdata", "example.json"),
				expected: types.DockerAuthConfig{
					Username: "example",
					Password: "org",
				},
			},
			{
				name:     "match none",
				hostname: "registry.example.org",
				path:     filepath.Join("testdata", "example.json"),
			},
			{
				name:     "match docker.io",
				hostname: "docker.io",
				path:     filepath.Join("testdata", "full.json"),
				expected: types.DockerAuthConfig{
					Username: "docker",
					Password: "io",
				},
			},
			{
				name:     "match docker.io normalized",
				hostname: "docker.io",
				path:     filepath.Join("testdata", "abnormal.json"),
				expected: types.DockerAuthConfig{
					Username: "index",
					Password: "docker.io",
				},
			},
			{
				name:     "normalize registry",
				hostname: "https://example.org/v1",
				path:     filepath.Join("testdata", "full.json"),
				expected: types.DockerAuthConfig{
					Username: "example",
					Password: "org",
				},
			},
			{
				name:     "match localhost",
				hostname: "http://localhost",
				path:     filepath.Join("testdata", "full.json"),
				expected: types.DockerAuthConfig{
					Username: "local",
					Password: "host",
				},
			},
			{
				name:     "match ip",
				hostname: "10.10.30.45:5000",
				path:     filepath.Join("testdata", "full.json"),
				expected: types.DockerAuthConfig{
					Username: "10.10",
					Password: "30.45-5000",
				},
			},
			{
				name:     "match port",
				hostname: "https://localhost:5000",
				path:     filepath.Join("testdata", "abnormal.json"),
				expected: types.DockerAuthConfig{
					Username: "local",
					Password: "host-5000",
				},
			},
			{
				name:     "use system context",
				hostname: "example.org",
				path:     filepath.Join("testdata", "example.json"),
				expected: types.DockerAuthConfig{
					Username: "foo",
					Password: "bar",
				},
				sys: &types.SystemContext{
					DockerAuthConfig: &types.DockerAuthConfig{
						Username: "foo",
						Password: "bar",
					},
				},
			},
			{
				name:     "identity token",
				hostname: "example.org",
				path:     filepath.Join("testdata", "example_identitytoken.json"),
				expected: types.DockerAuthConfig{
					Username:      "00000000-0000-0000-0000-000000000000",
					Password:      "",
					IdentityToken: "some very long identity token",
				},
			},
			{
				name:     "match none (empty.json)",
				hostname: "https://localhost:5000",
				path:     filepath.Join("testdata", "empty.json"),
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if err := os.RemoveAll(configPath); err != nil {
					t.Fatal(err)
				}

				if tc.path != "" {
					contents, err := ioutil.ReadFile(tc.path)
					if err != nil {
						t.Fatal(err)
					}

					if err := ioutil.WriteFile(configPath, contents, 0640); err != nil {
						t.Fatal(err)
					}
				}

				var sys *types.SystemContext
				if tc.sys != nil {
					sys = tc.sys
				}
				auth, err := GetCredentials(sys, tc.hostname)
				assert.Equal(t, tc.expectedError, err)
				assert.Equal(t, tc.expected, auth)

				// Test for the previous APIs.
				username, password, err := GetAuthentication(sys, tc.hostname)
				if tc.expected.IdentityToken != "" {
					assert.Equal(t, "", username)
					assert.Equal(t, "", password)
					assert.Error(t, err)
				} else {
					assert.Equal(t, tc.expected.Username, username)
					assert.Equal(t, tc.expected.Password, password)
					assert.Equal(t, tc.expectedError, err)
				}

				require.NoError(t, os.RemoveAll(configPath))
			})
		}
	}
}

func TestGetAuthFromLegacyFile(t *testing.T) {
	origHomeDir := homedir.Get()
	tmpDir, err := ioutil.TempDir("", "test_docker_client_get_auth")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("using temporary home directory: %q", tmpDir)
	// override homedir
	os.Setenv(homedir.Key(), tmpDir)
	defer func() {
		err := os.RemoveAll(tmpDir)
		if err != nil {
			t.Logf("failed to cleanup temporary home directory %q: %v", tmpDir, err)
		}
		os.Setenv(homedir.Key(), origHomeDir)
	}()

	configPath := filepath.Join(tmpDir, ".dockercfg")
	contents, err := ioutil.ReadFile(filepath.Join("testdata", "legacy.json"))
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name          string
		hostname      string
		expected      types.DockerAuthConfig
		expectedError error
	}{
		{
			name:     "normalize registry",
			hostname: "https://docker.io/v1",
			expected: types.DockerAuthConfig{
				Username: "docker",
				Password: "io-legacy",
			},
		},
		{
			name:     "ignore schema and path",
			hostname: "http://index.docker.io/v1",
			expected: types.DockerAuthConfig{
				Username: "docker",
				Password: "io-legacy",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := ioutil.WriteFile(configPath, contents, 0640); err != nil {
				t.Fatal(err)
			}

			auth, err := GetCredentials(nil, tc.hostname)
			assert.Equal(t, tc.expectedError, err)
			assert.Equal(t, tc.expected, auth)

			// Testing for previous APIs
			username, password, err := GetAuthentication(nil, tc.hostname)
			assert.Equal(t, tc.expectedError, err)
			assert.Equal(t, tc.expected.Username, username)
			assert.Equal(t, tc.expected.Password, password)
		})
	}
}

func TestGetAuthPreferNewConfig(t *testing.T) {
	origHomeDir := homedir.Get()
	tmpDir, err := ioutil.TempDir("", "test_docker_client_get_auth")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("using temporary home directory: %q", tmpDir)
	// override homedir
	os.Setenv(homedir.Key(), tmpDir)
	defer func() {
		err := os.RemoveAll(tmpDir)
		if err != nil {
			t.Logf("failed to cleanup temporary home directory %q: %v", tmpDir, err)
		}
		os.Setenv(homedir.Key(), origHomeDir)
	}()

	configDir := filepath.Join(tmpDir, ".docker")
	if err := os.Mkdir(configDir, 0750); err != nil {
		t.Fatal(err)
	}

	for _, data := range []struct {
		source string
		target string
	}{
		{
			source: filepath.Join("testdata", "full.json"),
			target: filepath.Join(configDir, "config.json"),
		},
		{
			source: filepath.Join("testdata", "legacy.json"),
			target: filepath.Join(tmpDir, ".dockercfg"),
		},
	} {
		contents, err := ioutil.ReadFile(data.source)
		if err != nil {
			t.Fatal(err)
		}

		if err := ioutil.WriteFile(data.target, contents, 0640); err != nil {
			t.Fatal(err)
		}
	}

	auth, err := GetCredentials(nil, "docker.io")
	assert.NoError(t, err)
	assert.Equal(t, "docker", auth.Username)
	assert.Equal(t, "io", auth.Password)
}

func TestGetAuthFailsOnBadInput(t *testing.T) {
	origXDG := os.Getenv("XDG_RUNTIME_DIR")
	tmpXDGRuntimeDir, err := ioutil.TempDir("", "test_docker_client_get_auth")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("using temporary XDG_RUNTIME_DIR directory: %q", tmpXDGRuntimeDir)
	// override homedir
	os.Setenv("XDG_RUNTIME_DIR", tmpXDGRuntimeDir)
	defer func() {
		err := os.RemoveAll(tmpXDGRuntimeDir)
		if err != nil {
			t.Logf("failed to cleanup temporary home directory %q: %v", tmpXDGRuntimeDir, err)
		}
		os.Setenv("XDG_RUNTIME_DIR", origXDG)
	}()

	origHomeDir := homedir.Get()
	tmpHomeDir, err := ioutil.TempDir("", "test_docker_client_get_auth")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("using temporary home directory: %q", tmpHomeDir)
	// override homedir
	os.Setenv(homedir.Key(), tmpHomeDir)
	defer func() {
		err := os.RemoveAll(tmpHomeDir)
		if err != nil {
			t.Logf("failed to cleanup temporary home directory %q: %v", tmpHomeDir, err)
		}
		os.Setenv(homedir.Key(), origHomeDir)
	}()

	configDir := filepath.Join(tmpXDGRuntimeDir, "containers")
	if err := os.Mkdir(configDir, 0750); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "auth.json")

	// no config file present
	auth, err := GetCredentials(nil, "index.docker.io")
	if err != nil {
		t.Fatalf("got unexpected error: %#+v", err)
	}
	assert.Equal(t, types.DockerAuthConfig{}, auth)

	if err := ioutil.WriteFile(configPath, []byte("Json rocks! Unless it doesn't."), 0640); err != nil {
		t.Fatalf("failed to write file %q: %v", configPath, err)
	}
	auth, err = GetCredentials(nil, "index.docker.io")
	if err == nil {
		t.Fatalf("got unexpected non-error: username=%q, password=%q", auth.Username, auth.Password)
	}
	if _, ok := errors.Cause(err).(*json.SyntaxError); !ok {
		t.Fatalf("expected JSON syntax error, not: %#+v", err)
	}

	// remove the invalid config file
	os.RemoveAll(configPath)
	// no config file present
	auth, err = GetCredentials(nil, "index.docker.io")
	if err != nil {
		t.Fatalf("got unexpected error: %#+v", err)
	}
	assert.Equal(t, types.DockerAuthConfig{}, auth)

	configPath = filepath.Join(tmpHomeDir, ".dockercfg")
	if err := ioutil.WriteFile(configPath, []byte("I'm certainly not a json string."), 0640); err != nil {
		t.Fatalf("failed to write file %q: %v", configPath, err)
	}
	auth, err = GetCredentials(nil, "index.docker.io")
	if err == nil {
		t.Fatalf("got unexpected non-error: username=%q, password=%q", auth.Username, auth.Password)
	}
	if _, ok := errors.Cause(err).(*json.SyntaxError); !ok {
		t.Fatalf("expected JSON syntax error, not: %#+v", err)
	}
}

func TestGetAllCredentials(t *testing.T) {
	// Create a temporary authentication file.
	tmpFile, err := ioutil.TempFile("", "auth.json.")
	require.NoError(t, err)
	_, err = tmpFile.Write([]byte{'{', '}'})
	require.NoError(t, err)
	err = tmpFile.Close()
	require.NoError(t, err)
	authFilePath := tmpFile.Name()
	sys := types.SystemContext{AuthFilePath: authFilePath}

	data := []struct {
		server   string
		username string
		password string
	}{
		{
			server:   "example.org",
			username: "example-user",
			password: "example-password",
		},
		{
			server:   "quay.io",
			username: "quay-user",
			password: "quay-password",
		},
		{
			server:   "localhost:5000",
			username: "local-user",
			password: "local-password",
		},
	}

	// Write the credentials to the authfile.
	for _, d := range data {
		err := SetAuthentication(&sys, d.server, d.username, d.password)
		require.NoError(t, err)
	}

	// Now ask for all credentials and make sure that map includes all
	// servers and the correct credentials.
	authConfigs, err := GetAllCredentials(&sys)
	require.NoError(t, err)
	assert.Equal(t, len(data), len(authConfigs))
	for _, d := range data {
		conf, exists := authConfigs[d.server]
		assert.True(t, exists)
		assert.Equal(t, d.username, conf.Username)
		assert.Equal(t, d.password, conf.Password)
	}

}

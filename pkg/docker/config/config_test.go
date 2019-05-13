package config

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/containers/image/types"
	"github.com/containers/storage/pkg/homedir"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPathToAuth(t *testing.T) {
	uid := fmt.Sprintf("%d", os.Getuid())

	tmpDir, err := ioutil.TempDir("", "TestGetPathToAuth")
	require.NoError(t, err)

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
		sys      *types.SystemContext
		xrd      string
		expected string
	}{
		// Default paths
		{&types.SystemContext{}, "", "/run/containers/" + uid + "/auth.json"},
		{nil, "", "/run/containers/" + uid + "/auth.json"},
		// SystemContext overrides
		{&types.SystemContext{AuthFilePath: "/absolute/path"}, "", "/absolute/path"},
		{&types.SystemContext{RootForImplicitAbsolutePaths: "/prefix"}, "", "/prefix/run/containers/" + uid + "/auth.json"},
		// XDG_RUNTIME_DIR defined
		{nil, tmpDir, tmpDir + "/containers/auth.json"},
		{nil, tmpDir + "/thisdoesnotexist", ""},
	} {
		if c.xrd != "" {
			os.Setenv("XDG_RUNTIME_DIR", c.xrd)
		} else {
			os.Unsetenv("XDG_RUNTIME_DIR")
		}
		res, err := getPathToAuth(c.sys)
		if c.expected == "" {
			assert.Error(t, err)
		} else {
			require.NoError(t, err)
			assert.Equal(t, c.expected, res)
		}
	}
}

func TestGetAuth(t *testing.T) {
	origXDG := os.Getenv("XDG_RUNTIME_DIR")
	tmpDir1, err := ioutil.TempDir("", "test_docker_client_get_auth")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("using temporary XDG_RUNTIME_DIR directory: %q", tmpDir1)
	// override XDG_RUNTIME_DIR
	os.Setenv("XDG_RUNTIME_DIR", tmpDir1)
	defer func() {
		err := os.RemoveAll(tmpDir1)
		if err != nil {
			t.Logf("failed to cleanup temporary home directory %q: %v", tmpDir1, err)
		}
		os.Setenv("XDG_RUNTIME_DIR", origXDG)
	}()

	origHomeDir := homedir.Get()
	tmpDir2, err := ioutil.TempDir("", "test_docker_client_get_auth")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("using temporary home directory: %q", tmpDir2)
	//override homedir
	os.Setenv(homedir.Key(), tmpDir2)
	defer func() {
		err := os.RemoveAll(tmpDir2)
		if err != nil {
			t.Logf("failed to cleanup temporary home directory %q: %v", tmpDir2, err)
		}
		os.Setenv(homedir.Key(), origHomeDir)
	}()

	configDir1 := filepath.Join(tmpDir1, "containers")
	if err := os.MkdirAll(configDir1, 0700); err != nil {
		t.Fatal(err)
	}
	configDir2 := filepath.Join(tmpDir2, ".docker")
	if err := os.MkdirAll(configDir2, 0700); err != nil {
		t.Fatal(err)
	}
	configPaths := [2]string{filepath.Join(configDir1, "auth.json"), filepath.Join(configDir2, "config.json")}

	for _, configPath := range configPaths {
		for _, tc := range []struct {
			name             string
			hostname         string
			authConfig       testAuthConfig
			expectedUsername string
			expectedPassword string
			expectedError    error
			sys              *types.SystemContext
		}{
			{
				name:       "empty hostname",
				authConfig: makeTestAuthConfig(testAuthConfigDataMap{"localhost:5000": testAuthConfigData{"bob", "password"}}),
			},
			{
				name:     "no auth config",
				hostname: "index.docker.io",
			},
			{
				name:             "match one",
				hostname:         "example.org",
				authConfig:       makeTestAuthConfig(testAuthConfigDataMap{"example.org": testAuthConfigData{"joe", "mypass"}}),
				expectedUsername: "joe",
				expectedPassword: "mypass",
			},
			{
				name:       "match none",
				hostname:   "registry.example.org",
				authConfig: makeTestAuthConfig(testAuthConfigDataMap{"example.org": testAuthConfigData{"joe", "mypass"}}),
			},
			{
				name:     "match docker.io",
				hostname: "docker.io",
				authConfig: makeTestAuthConfig(testAuthConfigDataMap{
					"example.org":     testAuthConfigData{"example", "org"},
					"index.docker.io": testAuthConfigData{"index", "docker.io"},
					"docker.io":       testAuthConfigData{"docker", "io"},
				}),
				expectedUsername: "docker",
				expectedPassword: "io",
			},
			{
				name:     "match docker.io normalized",
				hostname: "docker.io",
				authConfig: makeTestAuthConfig(testAuthConfigDataMap{
					"example.org":                testAuthConfigData{"bob", "pw"},
					"https://index.docker.io/v1": testAuthConfigData{"alice", "wp"},
				}),
				expectedUsername: "alice",
				expectedPassword: "wp",
			},
			{
				name:     "normalize registry",
				hostname: "https://docker.io/v1",
				authConfig: makeTestAuthConfig(testAuthConfigDataMap{
					"docker.io":      testAuthConfigData{"user", "pw"},
					"localhost:5000": testAuthConfigData{"joe", "pass"},
				}),
				expectedUsername: "user",
				expectedPassword: "pw",
			},
			{
				name:     "match localhost",
				hostname: "http://localhost",
				authConfig: makeTestAuthConfig(testAuthConfigDataMap{
					"docker.io":   testAuthConfigData{"user", "pw"},
					"localhost":   testAuthConfigData{"joe", "pass"},
					"example.com": testAuthConfigData{"alice", "pwd"},
				}),
				expectedUsername: "joe",
				expectedPassword: "pass",
			},
			{
				name:     "match ip",
				hostname: "10.10.3.56:5000",
				authConfig: makeTestAuthConfig(testAuthConfigDataMap{
					"10.10.30.45":     testAuthConfigData{"user", "pw"},
					"localhost":       testAuthConfigData{"joe", "pass"},
					"10.10.3.56":      testAuthConfigData{"alice", "pwd"},
					"10.10.3.56:5000": testAuthConfigData{"me", "mine"},
				}),
				expectedUsername: "me",
				expectedPassword: "mine",
			},
			{
				name:     "match port",
				hostname: "https://localhost:5000",
				authConfig: makeTestAuthConfig(testAuthConfigDataMap{
					"https://127.0.0.1:5000": testAuthConfigData{"user", "pw"},
					"http://localhost":       testAuthConfigData{"joe", "pass"},
					"https://localhost:5001": testAuthConfigData{"alice", "pwd"},
					"localhost:5000":         testAuthConfigData{"me", "mine"},
				}),
				expectedUsername: "me",
				expectedPassword: "mine",
			},
			{
				name:     "use system context",
				hostname: "example.org",
				authConfig: makeTestAuthConfig(testAuthConfigDataMap{
					"example.org": testAuthConfigData{"user", "pw"},
				}),
				expectedUsername: "foo",
				expectedPassword: "bar",
				sys: &types.SystemContext{
					DockerAuthConfig: &types.DockerAuthConfig{
						Username: "foo",
						Password: "bar",
					},
				},
			},
		} {
			contents, err := json.MarshalIndent(&tc.authConfig, "", "  ")
			if err != nil {
				t.Errorf("[%s] failed to marshal authConfig: %v", tc.name, err)
				continue
			}
			if err := ioutil.WriteFile(configPath, contents, 0640); err != nil {
				t.Errorf("[%s] failed to write file %q: %v", tc.name, configPath, err)
				continue
			}

			var sys *types.SystemContext
			if tc.sys != nil {
				sys = tc.sys
			}
			username, password, err := GetAuthentication(sys, tc.hostname)
			if err == nil && tc.expectedError != nil {
				t.Errorf("[%s] got unexpected non error and username=%q, password=%q", tc.name, username, password)
				continue
			}
			if err != nil && tc.expectedError == nil {
				t.Errorf("[%s] got unexpected error: %#+v", tc.name, err)
				continue
			}
			if !reflect.DeepEqual(err, tc.expectedError) {
				t.Errorf("[%s] got unexpected error: %#+v != %#+v", tc.name, err, tc.expectedError)
				continue
			}

			if username != tc.expectedUsername {
				t.Errorf("[%s] got unexpected user name: %q != %q", tc.name, username, tc.expectedUsername)
			}
			if password != tc.expectedPassword {
				t.Errorf("[%s] got unexpected user name: %q != %q", tc.name, password, tc.expectedPassword)
			}
		}
		os.RemoveAll(configPath)
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

	for _, tc := range []struct {
		name             string
		hostname         string
		authConfig       testAuthConfig
		expectedUsername string
		expectedPassword string
		expectedError    error
	}{
		{
			name:     "normalize registry",
			hostname: "https://docker.io/v1",
			authConfig: makeTestAuthConfig(testAuthConfigDataMap{
				"docker.io":      testAuthConfigData{"user", "pw"},
				"localhost:5000": testAuthConfigData{"joe", "pass"},
			}),
			expectedUsername: "user",
			expectedPassword: "pw",
		},
		{
			name:     "ignore schema and path",
			hostname: "http://index.docker.io/v1",
			authConfig: makeTestAuthConfig(testAuthConfigDataMap{
				"docker.io/v2":         testAuthConfigData{"user", "pw"},
				"https://localhost/v1": testAuthConfigData{"joe", "pwd"},
			}),
			expectedUsername: "user",
			expectedPassword: "pw",
		},
	} {
		contents, err := json.MarshalIndent(&tc.authConfig.Auths, "", "  ")
		if err != nil {
			t.Errorf("[%s] failed to marshal authConfig: %v", tc.name, err)
			continue
		}
		if err := ioutil.WriteFile(configPath, contents, 0640); err != nil {
			t.Errorf("[%s] failed to write file %q: %v", tc.name, configPath, err)
			continue
		}

		username, password, err := GetAuthentication(nil, tc.hostname)
		if err == nil && tc.expectedError != nil {
			t.Errorf("[%s] got unexpected non error and username=%q, password=%q", tc.name, username, password)
			continue
		}
		if err != nil && tc.expectedError == nil {
			t.Errorf("[%s] got unexpected error: %#+v", tc.name, err)
			continue
		}
		if !reflect.DeepEqual(err, tc.expectedError) {
			t.Errorf("[%s] got unexpected error: %#+v != %#+v", tc.name, err, tc.expectedError)
			continue
		}

		if username != tc.expectedUsername {
			t.Errorf("[%s] got unexpected user name: %q != %q", tc.name, username, tc.expectedUsername)
		}
		if password != tc.expectedPassword {
			t.Errorf("[%s] got unexpected user name: %q != %q", tc.name, password, tc.expectedPassword)
		}
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
		path string
		ac   interface{}
	}{
		{
			filepath.Join(configDir, "config.json"),
			makeTestAuthConfig(testAuthConfigDataMap{
				"https://index.docker.io/v1/": testAuthConfigData{"alice", "pass"},
			}),
		},
		{
			filepath.Join(tmpDir, ".dockercfg"),
			makeTestAuthConfig(testAuthConfigDataMap{
				"https://index.docker.io/v1/": testAuthConfigData{"bob", "pw"},
			}).Auths,
		},
	} {
		contents, err := json.MarshalIndent(&data.ac, "", "  ")
		if err != nil {
			t.Fatalf("failed to marshal authConfig: %v", err)
		}
		if err := ioutil.WriteFile(data.path, contents, 0640); err != nil {
			t.Fatalf("failed to write file %q: %v", data.path, err)
		}
	}

	username, password, err := GetAuthentication(nil, "index.docker.io")
	if err != nil {
		t.Fatalf("got unexpected error: %#+v", err)
	}

	if username != "alice" {
		t.Fatalf("got unexpected user name: %q != %q", username, "alice")
	}
	if password != "pass" {
		t.Fatalf("got unexpected user name: %q != %q", password, "pass")
	}
}

func TestGetAuthFailsOnBadInput(t *testing.T) {
	origXDG := os.Getenv("XDG_RUNTIME_DIR")
	tmpDir1, err := ioutil.TempDir("", "test_docker_client_get_auth")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("using temporary XDG_RUNTIME_DIR directory: %q", tmpDir1)
	// override homedir
	os.Setenv("XDG_RUNTIME_DIR", tmpDir1)
	defer func() {
		err := os.RemoveAll(tmpDir1)
		if err != nil {
			t.Logf("failed to cleanup temporary home directory %q: %v", tmpDir1, err)
		}
		os.Setenv("XDG_RUNTIME_DIR", origXDG)
	}()

	origHomeDir := homedir.Get()
	tmpDir2, err := ioutil.TempDir("", "test_docker_client_get_auth")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("using temporary home directory: %q", tmpDir2)
	// override homedir
	os.Setenv(homedir.Key(), tmpDir2)
	defer func() {
		err := os.RemoveAll(tmpDir2)
		if err != nil {
			t.Logf("failed to cleanup temporary home directory %q: %v", tmpDir2, err)
		}
		os.Setenv(homedir.Key(), origHomeDir)
	}()

	configDir := filepath.Join(tmpDir1, "containers")
	if err := os.Mkdir(configDir, 0750); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "auth.json")

	// no config file present
	username, password, err := GetAuthentication(nil, "index.docker.io")
	if err != nil {
		t.Fatalf("got unexpected error: %#+v", err)
	}
	if len(username) > 0 || len(password) > 0 {
		t.Fatalf("got unexpected not empty username/password: %q/%q", username, password)
	}

	if err := ioutil.WriteFile(configPath, []byte("Json rocks! Unless it doesn't."), 0640); err != nil {
		t.Fatalf("failed to write file %q: %v", configPath, err)
	}
	username, password, err = GetAuthentication(nil, "index.docker.io")
	if err == nil {
		t.Fatalf("got unexpected non-error: username=%q, password=%q", username, password)
	}
	if _, ok := errors.Cause(err).(*json.SyntaxError); !ok {
		t.Fatalf("expected JSON syntax error, not: %#+v", err)
	}

	// remove the invalid config file
	os.RemoveAll(configPath)
	// no config file present
	username, password, err = GetAuthentication(nil, "index.docker.io")
	if err != nil {
		t.Fatalf("got unexpected error: %#+v", err)
	}
	if len(username) > 0 || len(password) > 0 {
		t.Fatalf("got unexpected not empty username/password: %q/%q", username, password)
	}

	configPath = filepath.Join(tmpDir2, ".dockercfg")
	if err := ioutil.WriteFile(configPath, []byte("I'm certainly not a json string."), 0640); err != nil {
		t.Fatalf("failed to write file %q: %v", configPath, err)
	}
	username, password, err = GetAuthentication(nil, "index.docker.io")
	if err == nil {
		t.Fatalf("got unexpected non-error: username=%q, password=%q", username, password)
	}
	if _, ok := errors.Cause(err).(*json.SyntaxError); !ok {
		t.Fatalf("expected JSON syntax error, not: %#+v", err)
	}
}

type testAuthConfigData struct {
	username string
	password string
}

type testAuthConfigDataMap map[string]testAuthConfigData

type testAuthConfigEntry struct {
	Auth string `json:"auth,omitempty"`
}

type testAuthConfig struct {
	Auths map[string]testAuthConfigEntry `json:"auths"`
}

// encodeAuth creates an auth value from given authConfig data to be stored in auth config file.
// Inspired by github.com/docker/docker/cliconfig/config.go v1.10.3.
func encodeAuth(authConfig *testAuthConfigData) string {
	authStr := authConfig.username + ":" + authConfig.password
	msg := []byte(authStr)
	encoded := make([]byte, base64.StdEncoding.EncodedLen(len(msg)))
	base64.StdEncoding.Encode(encoded, msg)
	return string(encoded)
}

func makeTestAuthConfig(authConfigData map[string]testAuthConfigData) testAuthConfig {
	ac := testAuthConfig{
		Auths: make(map[string]testAuthConfigEntry),
	}
	for host, data := range authConfigData {
		ac.Auths[host] = testAuthConfigEntry{
			Auth: encodeAuth(&data),
		}
	}
	return ac
}

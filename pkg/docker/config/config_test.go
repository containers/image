package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPathToAuth(t *testing.T) {
	const linux = "linux"
	const darwin = "darwin"

	uid := fmt.Sprintf("%d", os.Getuid())
	// We don’t have to override the home directory for this because use of this path does not depend
	// on any state of the filesystem.
	darwinDefault := filepath.Join(os.Getenv("HOME"), ".config", "containers", "auth.json")

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
		os           string
		xrd          string
		expected     string
		legacyFormat bool
	}{
		// Default paths
		{&types.SystemContext{}, linux, "", "/run/containers/" + uid + "/auth.json", false},
		{&types.SystemContext{}, darwin, "", darwinDefault, false},
		{nil, linux, "", "/run/containers/" + uid + "/auth.json", false},
		{nil, darwin, "", darwinDefault, false},
		// SystemContext overrides
		{&types.SystemContext{AuthFilePath: "/absolute/path"}, linux, "", "/absolute/path", false},
		{&types.SystemContext{AuthFilePath: "/absolute/path"}, darwin, "", "/absolute/path", false},
		{&types.SystemContext{LegacyFormatAuthFilePath: "/absolute/path"}, linux, "", "/absolute/path", true},
		{&types.SystemContext{LegacyFormatAuthFilePath: "/absolute/path"}, darwin, "", "/absolute/path", true},
		{&types.SystemContext{RootForImplicitAbsolutePaths: "/prefix"}, linux, "", "/prefix/run/containers/" + uid + "/auth.json", false},
		{&types.SystemContext{RootForImplicitAbsolutePaths: "/prefix"}, darwin, "", "/prefix/run/containers/" + uid + "/auth.json", false},
		// XDG_RUNTIME_DIR defined
		{nil, linux, tmpDir, tmpDir + "/containers/auth.json", false},
		{nil, darwin, tmpDir, darwinDefault, false},
		{nil, linux, tmpDir + "/thisdoesnotexist", "", false},
		{nil, darwin, tmpDir + "/thisdoesnotexist", darwinDefault, false},
	} {
		if c.xrd != "" {
			os.Setenv("XDG_RUNTIME_DIR", c.xrd)
		} else {
			os.Unsetenv("XDG_RUNTIME_DIR")
		}
		res, lf, err := getPathToAuthWithOS(c.sys, c.os)
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

	// override PATH for executing credHelper
	curtDir, err := os.Getwd()
	require.NoError(t, err)
	origPath := os.Getenv("PATH")
	newPath := fmt.Sprintf("%s:%s", filepath.Join(curtDir, "testdata"), origPath)
	os.Setenv("PATH", newPath)
	t.Logf("using PATH: %q", newPath)
	defer func() {
		os.Setenv("PATH", origPath)
	}()

	tmpHomeDir, err := ioutil.TempDir("", "test_docker_client_get_auth")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("using temporary home directory: %q", tmpHomeDir)
	defer func() {
		err := os.RemoveAll(tmpHomeDir)
		if err != nil {
			t.Logf("failed to cleanup temporary home directory %q: %v", tmpHomeDir, err)
		}
	}()

	configDir1 := filepath.Join(tmpXDGRuntimeDir, "containers")
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		configDir1 = filepath.Join(tmpHomeDir, ".config", "containers")
	}
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
			name            string
			ref             string
			hostname        string
			path            string
			expected        types.DockerAuthConfig
			sys             *types.SystemContext
			testPreviousAPI bool
		}{
			{
				name:            "no auth config",
				hostname:        "index.docker.io",
				testPreviousAPI: true,
			},
			{
				name: "empty hostname",
				path: filepath.Join("testdata", "example.json"),
			},
			{
				name:            "match one",
				hostname:        "example.org",
				path:            filepath.Join("testdata", "example.json"),
				expected:        types.DockerAuthConfig{Username: "example", Password: "org"},
				testPreviousAPI: true,
			},
			{
				name:            "match none",
				hostname:        "registry.example.org",
				path:            filepath.Join("testdata", "example.json"),
				testPreviousAPI: true,
			},
			{
				name:            "match docker.io",
				hostname:        "docker.io",
				path:            filepath.Join("testdata", "full.json"),
				expected:        types.DockerAuthConfig{Username: "docker", Password: "io"},
				testPreviousAPI: true,
			},
			{
				name:            "match docker.io normalized",
				hostname:        "docker.io",
				path:            filepath.Join("testdata", "abnormal.json"),
				expected:        types.DockerAuthConfig{Username: "index", Password: "docker.io"},
				testPreviousAPI: true,
			},
			{
				name:            "normalize registry",
				hostname:        "normalize.example.org",
				path:            filepath.Join("testdata", "full.json"),
				expected:        types.DockerAuthConfig{Username: "normalize", Password: "example"},
				testPreviousAPI: true,
			},
			{
				name:            "match localhost",
				hostname:        "localhost",
				path:            filepath.Join("testdata", "full.json"),
				expected:        types.DockerAuthConfig{Username: "local", Password: "host"},
				testPreviousAPI: true,
			},
			{
				name:            "match ip",
				hostname:        "10.10.30.45:5000",
				path:            filepath.Join("testdata", "full.json"),
				expected:        types.DockerAuthConfig{Username: "10.10", Password: "30.45-5000"},
				testPreviousAPI: true,
			},
			{
				name:            "match port",
				hostname:        "localhost:5000",
				path:            filepath.Join("testdata", "abnormal.json"),
				expected:        types.DockerAuthConfig{Username: "local", Password: "host-5000"},
				testPreviousAPI: true,
			},
			{
				name:     "use system context",
				hostname: "example.org",
				path:     filepath.Join("testdata", "example.json"),
				expected: types.DockerAuthConfig{Username: "foo", Password: "bar"},
				sys: &types.SystemContext{
					DockerAuthConfig: &types.DockerAuthConfig{
						Username: "foo",
						Password: "bar",
					},
				},
				testPreviousAPI: true,
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
				testPreviousAPI: true,
			},
			{
				name:            "match none (empty.json)",
				hostname:        "https://localhost:5000",
				path:            filepath.Join("testdata", "empty.json"),
				testPreviousAPI: true,
			},
			{
				name:     "credhelper from registries.conf",
				hostname: "registry-a.com",
				sys: &types.SystemContext{
					SystemRegistriesConfPath:    filepath.Join("testdata", "cred-helper.conf"),
					SystemRegistriesConfDirPath: filepath.Join("testdata", "IdoNotExist"),
				},
				expected:        types.DockerAuthConfig{Username: "foo", Password: "bar"},
				testPreviousAPI: true,
			},
			{
				name:     "identity token credhelper from registries.conf",
				hostname: "registry-b.com",
				sys: &types.SystemContext{
					SystemRegistriesConfPath:    filepath.Join("testdata", "cred-helper.conf"),
					SystemRegistriesConfDirPath: filepath.Join("testdata", "IdoNotExist"),
				},
				expected:        types.DockerAuthConfig{IdentityToken: "fizzbuzz"},
				testPreviousAPI: true,
			},
			{
				name:            "match ref image",
				hostname:        "example.org",
				ref:             "example.org/repo/image:latest",
				path:            filepath.Join("testdata", "refpath.json"),
				expected:        types.DockerAuthConfig{Username: "example", Password: "org"},
				testPreviousAPI: false,
			},
			{
				name:            "match ref repo",
				hostname:        "example.org",
				ref:             "example.org/repo",
				path:            filepath.Join("testdata", "refpath.json"),
				expected:        types.DockerAuthConfig{Username: "example", Password: "org"},
				testPreviousAPI: false,
			},
			{
				name:            "match ref host",
				hostname:        "example.org",
				ref:             "example.org/image:latest",
				path:            filepath.Join("testdata", "refpath.json"),
				expected:        types.DockerAuthConfig{Username: "local", Password: "host"},
				testPreviousAPI: false,
			},
			// Test matching of docker.io/[library/] explicitly, to make sure the docker.io
			// normalization behavior doesn’t affect the semantics.
			{
				name:            "docker.io library repo match",
				hostname:        "docker.io",
				ref:             "docker.io/library/busybox:latest",
				path:            filepath.Join("testdata", "refpath.json"),
				expected:        types.DockerAuthConfig{Username: "library", Password: "busybox"},
				testPreviousAPI: false,
			},
			{
				name:            "docker.io library namespace match",
				hostname:        "docker.io",
				ref:             "docker.io/library/notbusybox:latest",
				path:            filepath.Join("testdata", "refpath.json"),
				expected:        types.DockerAuthConfig{Username: "library", Password: "other"},
				testPreviousAPI: false,
			},
			{ // This tests that the docker.io/vendor key in auth file is not normalized to docker.io/library/vendor
				name:            "docker.io vendor repo match",
				hostname:        "docker.io",
				ref:             "docker.io/vendor/product:latest",
				path:            filepath.Join("testdata", "refpath.json"),
				expected:        types.DockerAuthConfig{Username: "first", Password: "level"},
				testPreviousAPI: false,
			},
			// ref: "docker.io/vendor:latest" is imposible to express using the reference syntax,
			// it is normalized to "docker.io/library/vendor:latest".
			{
				name:            "docker.io host-only match",
				hostname:        "docker.io",
				ref:             "docker.io/other-vendor/other-product:latest",
				path:            filepath.Join("testdata", "refpath.json"),
				expected:        types.DockerAuthConfig{Username: "top", Password: "level"},
				testPreviousAPI: false,
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

				var ref reference.Named
				if tc.ref != "" {
					ref, err = reference.ParseNamed(tc.ref)
					require.NoError(t, err)
				}

				auth, err := getCredentialsWithHomeDir(sys, ref, tc.hostname, tmpHomeDir)
				require.NoError(t, err)
				assert.Equal(t, tc.expected, auth)

				// Test for the previous APIs.
				if tc.testPreviousAPI {
					username, password, err := getAuthenticationWithHomeDir(sys, tc.hostname, tmpHomeDir)
					if tc.expected.IdentityToken != "" {
						assert.Error(t, err)
					} else {
						require.NoError(t, err)
						assert.Equal(t, tc.expected.Username, username)
						assert.Equal(t, tc.expected.Password, password)
					}
				}

				require.NoError(t, os.RemoveAll(configPath))
			})
		}
	}
}

func TestGetAuthFromLegacyFile(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "test_docker_client_get_auth")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("using temporary home directory: %q", tmpDir)
	defer func() {
		err := os.RemoveAll(tmpDir)
		if err != nil {
			t.Logf("failed to cleanup temporary home directory %q: %v", tmpDir, err)
		}
	}()

	configPath := filepath.Join(tmpDir, ".dockercfg")
	contents, err := ioutil.ReadFile(filepath.Join("testdata", "legacy.json"))
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name     string
		hostname string
		expected types.DockerAuthConfig
	}{
		{
			name:     "ignore schema and path",
			hostname: "localhost",
			expected: types.DockerAuthConfig{
				Username: "local",
				Password: "host-legacy",
			},
		},
		{
			name:     "normalize registry",
			hostname: "docker.io",
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

			auth, err := getCredentialsWithHomeDir(nil, nil, tc.hostname, tmpDir)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, auth)

			// Testing for previous APIs
			username, password, err := getAuthenticationWithHomeDir(nil, tc.hostname, tmpDir)
			require.NoError(t, err)
			assert.Equal(t, tc.expected.Username, username)
			assert.Equal(t, tc.expected.Password, password)
		})
	}
}

func TestGetAuthPreferNewConfig(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "test_docker_client_get_auth")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("using temporary home directory: %q", tmpDir)
	defer func() {
		err := os.RemoveAll(tmpDir)
		if err != nil {
			t.Logf("failed to cleanup temporary home directory %q: %v", tmpDir, err)
		}
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

	auth, err := getCredentialsWithHomeDir(nil, nil, "docker.io", tmpDir)
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
	// override XDG_RUNTIME_DIR
	os.Setenv("XDG_RUNTIME_DIR", tmpXDGRuntimeDir)
	defer func() {
		err := os.RemoveAll(tmpXDGRuntimeDir)
		if err != nil {
			t.Logf("failed to cleanup temporary home directory %q: %v", tmpXDGRuntimeDir, err)
		}
		os.Setenv("XDG_RUNTIME_DIR", origXDG)
	}()

	tmpHomeDir, err := ioutil.TempDir("", "test_docker_client_get_auth")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("using temporary home directory: %q", tmpHomeDir)
	defer func() {
		err := os.RemoveAll(tmpHomeDir)
		if err != nil {
			t.Logf("failed to cleanup temporary home directory %q: %v", tmpHomeDir, err)
		}
	}()

	configDir := filepath.Join(tmpXDGRuntimeDir, "containers")
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		configDir = filepath.Join(tmpHomeDir, ".config", "containers")
	}
	if err := os.MkdirAll(configDir, 0750); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "auth.json")

	// no config file present
	auth, err := getCredentialsWithHomeDir(nil, nil, "index.docker.io", tmpHomeDir)
	if err != nil {
		t.Fatalf("got unexpected error: %#+v", err)
	}
	assert.Equal(t, types.DockerAuthConfig{}, auth)

	if err := ioutil.WriteFile(configPath, []byte("Json rocks! Unless it doesn't."), 0640); err != nil {
		t.Fatalf("failed to write file %q: %v", configPath, err)
	}
	auth, err = getCredentialsWithHomeDir(nil, nil, "index.docker.io", tmpHomeDir)
	if err == nil {
		t.Fatalf("got unexpected non-error: username=%q, password=%q", auth.Username, auth.Password)
	}
	if !strings.Contains(err.Error(), "unmarshaling JSON") {
		t.Fatalf("expected JSON syntax error, not: %#+v", err)
	}

	// remove the invalid config file
	os.RemoveAll(configPath)
	// no config file present
	auth, err = getCredentialsWithHomeDir(nil, nil, "index.docker.io", tmpHomeDir)
	if err != nil {
		t.Fatalf("got unexpected error: %#+v", err)
	}
	assert.Equal(t, types.DockerAuthConfig{}, auth)

	configPath = filepath.Join(tmpHomeDir, ".dockercfg")
	if err := ioutil.WriteFile(configPath, []byte("I'm certainly not a json string."), 0640); err != nil {
		t.Fatalf("failed to write file %q: %v", configPath, err)
	}
	auth, err = getCredentialsWithHomeDir(nil, nil, "index.docker.io", tmpHomeDir)
	if err == nil {
		t.Fatalf("got unexpected non-error: username=%q, password=%q", auth.Username, auth.Password)
	}
	if !strings.Contains(err.Error(), "unmarshaling JSON") {
		t.Fatalf("expected JSON syntax error, not: %#+v", err)
	}
}

func TestGetAllCredentials(t *testing.T) {
	// Create a temporary authentication file.
	tmpFile, err := ioutil.TempFile("", "auth.json.")
	require.NoError(t, err)
	authFilePath := tmpFile.Name()
	defer tmpFile.Close()
	defer os.Remove(authFilePath)
	// override PATH for executing credHelper
	path, err := os.Getwd()
	require.NoError(t, err)
	origPath := os.Getenv("PATH")
	newPath := fmt.Sprintf("%s:%s", filepath.Join(path, "testdata"), origPath)
	os.Setenv("PATH", newPath)
	t.Logf("using PATH: %q", newPath)
	defer func() {
		os.Setenv("PATH", origPath)
	}()
	err = os.Chmod(filepath.Join(path, "testdata", "docker-credential-helper-registry"), os.ModePerm)
	require.NoError(t, err)
	sys := types.SystemContext{
		AuthFilePath:                authFilePath,
		SystemRegistriesConfPath:    filepath.Join("testdata", "cred-helper-with-auth-files.conf"),
		SystemRegistriesConfDirPath: filepath.Join("testdata", "IdoNotExist"),
	}

	for _, data := range [][]struct {
		writeKey    string
		expectedKey string
		username    string
		password    string
	}{
		{
			{
				writeKey:    "example.org",
				expectedKey: "example.org",
				username:    "example-user",
				password:    "example-password",
			},
			{
				writeKey:    "quay.io",
				expectedKey: "quay.io",
				username:    "quay-user",
				password:    "quay-password",
			},
			{
				writeKey:    "localhost:5000",
				expectedKey: "localhost:5000",
				username:    "local-user",
				password:    "local-password",
			},
			{
				writeKey:    "",
				expectedKey: "registry-a.com",
				username:    "foo",
				password:    "bar",
			},
		},
	} {
		// Write the credentials to the authfile.
		err := ioutil.WriteFile(authFilePath, []byte{'{', '}'}, 0700)
		require.NoError(t, err)

		for _, d := range data {
			if d.writeKey == "" {
				continue
			}
			err := SetAuthentication(&sys, d.writeKey, d.username, d.password)
			require.NoError(t, err)
		}

		// Now ask for all credentials and make sure that map includes all
		// servers and the correct credentials.
		authConfigs, err := GetAllCredentials(&sys)
		require.NoError(t, err)
		require.Equal(t, len(data), len(authConfigs))

		for _, d := range data {
			conf, exists := authConfigs[d.expectedKey]
			require.True(t, exists, "%v", d)
			require.Equal(t, d.username, conf.Username, "%v", d)
			require.Equal(t, d.password, conf.Password, "%v", d)
		}
	}
}

func TestAuthKeysForRef(t *testing.T) {
	for _, tc := range []struct {
		name, ref string
		expected  []string
	}{
		{
			name: "image without tag",
			ref:  "quay.io/image",
			expected: []string{
				"quay.io/image",
				"quay.io",
			},
		},
		{
			name: "image with tag",
			ref:  "quay.io/image:latest",
			expected: []string{
				"quay.io/image",
				"quay.io",
			},
		},
		{
			name: "image single path tag",
			ref:  "quay.io/user/image:latest",
			expected: []string{
				"quay.io/user/image",
				"quay.io/user",
				"quay.io",
			},
		},
		{
			name: "image with nested path",
			ref:  "quay.io/a/b/c/d/image:latest",
			expected: []string{
				"quay.io/a/b/c/d/image",
				"quay.io/a/b/c/d",
				"quay.io/a/b/c",
				"quay.io/a/b",
				"quay.io/a",
				"quay.io",
			},
		},
		{
			name: "docker.io library image",
			ref:  "docker.io/library/busybox:latest",
			expected: []string{
				"docker.io/library/busybox",
				"docker.io/library",
				"docker.io",
			},
		},
		{
			name: "docker.io non-library image",
			ref:  "docker.io/vendor/busybox:latest",
			expected: []string{
				"docker.io/vendor/busybox",
				"docker.io/vendor",
				"docker.io",
			},
		},
	} {
		ref, err := reference.ParseNamed(tc.ref)
		require.NoError(t, err, tc.name)

		result := authKeysForRef(ref)
		require.Equal(t, tc.expected, result, tc.name)
	}
}

func TestSetCredentials(t *testing.T) {
	const (
		usernamePrefix = "username-"
		passwordPrefix = "password-"
	)

	for _, tc := range [][]string{
		{"quay.io"},
		{"quay.io/a/b/c/d/image"},
		{
			"quay.io/a/b/c",
			"quay.io/a/b",
			"quay.io/a",
			"quay.io",
			"my-registry.local",
			"my-registry.local",
		},
		{
			"docker.io",
			"docker.io/vendor/product",
			"docker.io/vendor",
			"docker.io/library/busybox",
			"docker.io/library",
		},
	} {
		tmpFile, err := ioutil.TempFile("", "auth.json.set")
		require.NoError(t, err)
		defer os.RemoveAll(tmpFile.Name())

		_, err = tmpFile.WriteString("{}")
		require.NoError(t, err)
		sys := &types.SystemContext{AuthFilePath: tmpFile.Name()}

		writtenCredentials := map[string]int{}
		for i, input := range tc {
			_, err := SetCredentials(
				sys,
				input,
				usernamePrefix+fmt.Sprint(i),
				passwordPrefix+fmt.Sprint(i),
			)
			assert.NoError(t, err)
			writtenCredentials[input] = i // Possibly overwriting a previous entry
		}

		// Read the resulting file and verify it contains the expected keys
		auth, err := readJSONFile(tmpFile.Name(), false)
		require.NoError(t, err)
		assert.Len(t, auth.AuthConfigs, len(writtenCredentials))
		// auth.AuthConfigs and writtenCredentials are both maps, i.e. their keys are unique;
		// so A \subset B && len(A) == len(B) implies A == B
		for key := range writtenCredentials {
			assert.NotEmpty(t, auth.AuthConfigs[key].Auth)
		}

		// Verify that the configuration is interpreted as expected
		for key, i := range writtenCredentials {
			ref, err := reference.ParseNamed(key)
			// Full-registry keys and docker.io/top-level-namespace can't be read by GetCredentialsForRef
			if err == nil {
				auth, err := GetCredentialsForRef(sys, ref)
				require.NoError(t, err)
				assert.Equal(t, usernamePrefix+fmt.Sprint(i), auth.Username)
				assert.Equal(t, passwordPrefix+fmt.Sprint(i), auth.Password)
			}
		}
	}
}

func TestRemoveAuthentication(t *testing.T) {
	testAuth := dockerAuthConfig{Auth: "ZXhhbXBsZTpvcmc="}
	for _, tc := range []struct {
		config      dockerConfigFile
		inputs      []string
		shouldError bool
		assert      func(dockerConfigFile)
	}{
		{
			config: dockerConfigFile{
				AuthConfigs: map[string]dockerAuthConfig{
					"quay.io": testAuth,
				},
			},
			inputs: []string{"quay.io"},
			assert: func(auth dockerConfigFile) {
				assert.Len(t, auth.AuthConfigs, 0)
			},
		},
		{
			config: dockerConfigFile{
				AuthConfigs: map[string]dockerAuthConfig{
					"quay.io": testAuth,
				},
			},
			inputs:      []string{"quay.io/user/image"},
			shouldError: true, // not logged in
			assert: func(auth dockerConfigFile) {
				assert.Len(t, auth.AuthConfigs, 1)
				assert.NotEmpty(t, auth.AuthConfigs["quay.io"].Auth)
			},
		},
		{
			config: dockerConfigFile{
				AuthConfigs: map[string]dockerAuthConfig{
					"quay.io":           testAuth,
					"my-registry.local": testAuth,
				},
			},
			inputs: []string{"my-registry.local"},
			assert: func(auth dockerConfigFile) {
				assert.Len(t, auth.AuthConfigs, 1)
				assert.NotEmpty(t, auth.AuthConfigs["quay.io"].Auth)
			},
		},
		{
			config: dockerConfigFile{
				AuthConfigs: map[string]dockerAuthConfig{
					"quay.io/a/b/c":     testAuth,
					"quay.io/a/b":       testAuth,
					"quay.io/a":         testAuth,
					"quay.io":           testAuth,
					"my-registry.local": testAuth,
				},
			},
			inputs: []string{
				"quay.io/a/b",
				"quay.io",
				"my-registry.local",
			},
			assert: func(auth dockerConfigFile) {
				assert.Len(t, auth.AuthConfigs, 2)
				assert.NotEmpty(t, auth.AuthConfigs["quay.io/a/b/c"].Auth)
				assert.NotEmpty(t, auth.AuthConfigs["quay.io/a"].Auth)
			},
		},
	} {

		content, err := json.Marshal(&tc.config)
		require.NoError(t, err)

		tmpFile, err := ioutil.TempFile("", "auth.json")
		require.NoError(t, err)
		defer os.RemoveAll(tmpFile.Name())

		_, err = tmpFile.Write(content)
		require.NoError(t, err)

		sys := &types.SystemContext{AuthFilePath: tmpFile.Name()}

		for _, input := range tc.inputs {
			err := RemoveAuthentication(sys, input)
			if tc.shouldError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		}

		auth, err := readJSONFile(tmpFile.Name(), false)
		require.NoError(t, err)

		tc.assert(auth)
	}
}

func TestValidateKey(t *testing.T) {
	for _, tc := range []struct {
		key          string
		shouldError  bool
		isNamespaced bool
	}{
		{"my-registry.local", false, false},
		{"https://my-registry.local", true, false},
		{"my-registry.local/path", false, true},
		{"quay.io/a/b/c/d", false, true},
	} {

		isNamespaced, err := validateKey(tc.key)
		if tc.shouldError {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
		}
		assert.Equal(t, tc.isNamespaced, isNamespaced)
	}
}

func TestSetGetCredentials(t *testing.T) {
	const (
		username = "username"
		password = "password"
	)

	tmpDir, err := ioutil.TempDir("", "auth-test-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	for _, tc := range []struct {
		name         string
		set          string
		get          string
		useLegacyAPI bool
		shouldAuth   bool
	}{
		{
			name:       "Should match namespace",
			set:        "quay.io/foo",
			get:        "quay.io/foo/a",
			shouldAuth: true,
		},
		{
			name:       "Should match registry if repository provided",
			set:        "quay.io",
			get:        "quay.io/foo",
			shouldAuth: true,
		},
		{
			name:       "Should not match different repository",
			set:        "quay.io/foo",
			get:        "quay.io/bar",
			shouldAuth: false,
		},
		{
			name:       "Should match legacy registry entry (new API)",
			set:        "https://quay.io/v1/",
			get:        "quay.io/foo",
			shouldAuth: true,
		},
		{
			name:         "Should match legacy registry entry (legacy API)",
			set:          "https://quay.io/v1/",
			get:          "quay.io",
			shouldAuth:   true,
			useLegacyAPI: true,
		},
	} {

		// Create a new empty SystemContext referring an empty auth.json
		tmpFile, err := ioutil.TempFile("", "auth.json-")
		require.NoError(t, err)
		defer os.RemoveAll(tmpFile.Name())

		sys := &types.SystemContext{}
		if tc.useLegacyAPI {
			sys.LegacyFormatAuthFilePath = tmpFile.Name()
			_, err = tmpFile.WriteString(fmt.Sprintf(
				`{"%s":{"auth":"dXNlcm5hbWU6cGFzc3dvcmQ="}}`, tc.set,
			))
		} else {
			sys.AuthFilePath = tmpFile.Name()
			_, err = tmpFile.WriteString(fmt.Sprintf(
				`{"auths":{"%s":{"auth":"dXNlcm5hbWU6cGFzc3dvcmQ="}}}`, tc.set,
			))
		}
		require.NoError(t, err)

		// Try to authenticate against them
		var auth types.DockerAuthConfig
		if !tc.useLegacyAPI {
			ref, err := reference.ParseNamed(tc.get)
			require.NoError(t, err)
			auth, err = getCredentialsWithHomeDir(sys, ref, reference.Domain(ref), tmpDir)
			require.NoError(t, err)
		} else {
			auth, err = getCredentialsWithHomeDir(sys, nil, tc.get, tmpDir)
			require.NoError(t, err)
		}

		if tc.shouldAuth {
			assert.Equal(t, username, auth.Username, tc.name)
			assert.Equal(t, password, auth.Password, tc.name)
		} else {
			assert.Empty(t, auth.Username, tc.name)
			assert.Empty(t, auth.Password, tc.name)
		}
	}
}

package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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

	tmpDir := t.TempDir()

	for caseIndex, c := range []struct {
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
		t.Run(fmt.Sprintf("%d", caseIndex), func(t *testing.T) {
			// Always use t.Setenv() to ensure XDG_RUNTIME_DIR is restored to the original value after the test.
			// Then, in cases where the test needs XDG_RUNTIME_DIR unset (not just set to empty), use a raw os.Unsetenv()
			// to override the situation. (Sadly there isn’t a t.Unsetenv() as of Go 1.17.)
			t.Setenv("XDG_RUNTIME_DIR", c.xrd)
			if c.xrd == "" {
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
		})
	}
}

func TestGetAuth(t *testing.T) {
	tmpXDGRuntimeDir := t.TempDir()
	t.Logf("using temporary XDG_RUNTIME_DIR directory: %q", tmpXDGRuntimeDir)
	t.Setenv("XDG_RUNTIME_DIR", tmpXDGRuntimeDir)

	// override PATH for executing credHelper
	curtDir, err := os.Getwd()
	require.NoError(t, err)
	origPath := os.Getenv("PATH")
	newPath := fmt.Sprintf("%s:%s", filepath.Join(curtDir, "testdata"), origPath)
	t.Setenv("PATH", newPath)
	t.Logf("using PATH: %q", newPath)

	tmpHomeDir := t.TempDir()
	t.Logf("using temporary home directory: %q", tmpHomeDir)

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
			name     string
			key      string
			path     string
			expected types.DockerAuthConfig
			sys      *types.SystemContext
		}{
			{
				name:     "no auth config",
				key:      "index.docker.io",
				expected: types.DockerAuthConfig{},
			},
			{
				name:     "empty hostname",
				path:     filepath.Join("testdata", "example.json"),
				expected: types.DockerAuthConfig{},
			},
			{
				name:     "match one",
				key:      "example.org",
				path:     filepath.Join("testdata", "example.json"),
				expected: types.DockerAuthConfig{Username: "example", Password: "org"},
			},
			{
				name:     "match none",
				key:      "registry.example.org",
				path:     filepath.Join("testdata", "example.json"),
				expected: types.DockerAuthConfig{},
			},
			{
				name:     "match docker.io",
				key:      "docker.io",
				path:     filepath.Join("testdata", "full.json"),
				expected: types.DockerAuthConfig{Username: "docker", Password: "io"},
			},
			{
				name:     "match docker.io normalized",
				key:      "docker.io",
				path:     filepath.Join("testdata", "abnormal.json"),
				expected: types.DockerAuthConfig{Username: "index", Password: "docker.io"},
			},
			{
				name:     "normalize registry",
				key:      "normalize.example.org",
				path:     filepath.Join("testdata", "full.json"),
				expected: types.DockerAuthConfig{Username: "normalize", Password: "example"},
			},
			{
				name:     "match localhost",
				key:      "localhost",
				path:     filepath.Join("testdata", "full.json"),
				expected: types.DockerAuthConfig{Username: "local", Password: "host"},
			},
			{
				name:     "match ip",
				key:      "10.10.30.45:5000",
				path:     filepath.Join("testdata", "full.json"),
				expected: types.DockerAuthConfig{Username: "10.10", Password: "30.45-5000"},
			},
			{
				name:     "match port",
				key:      "localhost:5000",
				path:     filepath.Join("testdata", "abnormal.json"),
				expected: types.DockerAuthConfig{Username: "local", Password: "host-5000"},
			},
			{
				name:     "use system context",
				key:      "example.org",
				path:     filepath.Join("testdata", "example.json"),
				expected: types.DockerAuthConfig{Username: "foo", Password: "bar"},
				sys: &types.SystemContext{
					DockerAuthConfig: &types.DockerAuthConfig{
						Username: "foo",
						Password: "bar",
					},
				},
			},
			{
				name: "identity token",
				key:  "example.org",
				path: filepath.Join("testdata", "example_identitytoken.json"),
				expected: types.DockerAuthConfig{
					Username:      "00000000-0000-0000-0000-000000000000",
					Password:      "",
					IdentityToken: "some very long identity token",
				},
			},
			{
				name:     "match none (empty.json)",
				key:      "localhost:5000",
				path:     filepath.Join("testdata", "empty.json"),
				expected: types.DockerAuthConfig{},
			},
			{
				name: "credhelper from registries.conf",
				key:  "registry-a.com",
				sys: &types.SystemContext{
					SystemRegistriesConfPath:    filepath.Join("testdata", "cred-helper.conf"),
					SystemRegistriesConfDirPath: filepath.Join("testdata", "IdoNotExist"),
				},
				expected: types.DockerAuthConfig{Username: "foo", Password: "bar"},
			},
			{
				name: "identity token credhelper from registries.conf",
				key:  "registry-b.com",
				sys: &types.SystemContext{
					SystemRegistriesConfPath:    filepath.Join("testdata", "cred-helper.conf"),
					SystemRegistriesConfDirPath: filepath.Join("testdata", "IdoNotExist"),
				},
				expected: types.DockerAuthConfig{IdentityToken: "fizzbuzz"},
			},
			{
				name:     "match ref image",
				key:      "example.org/repo/image",
				path:     filepath.Join("testdata", "refpath.json"),
				expected: types.DockerAuthConfig{Username: "example", Password: "org"},
			},
			{
				name:     "match ref repo",
				key:      "example.org/repo",
				path:     filepath.Join("testdata", "refpath.json"),
				expected: types.DockerAuthConfig{Username: "example", Password: "org"},
			},
			{
				name:     "match ref host",
				key:      "example.org/image",
				path:     filepath.Join("testdata", "refpath.json"),
				expected: types.DockerAuthConfig{Username: "local", Password: "host"},
			},
			// Test matching of docker.io/[library/] explicitly, to make sure the docker.io
			// normalization behavior doesn’t affect the semantics.
			{
				name:     "docker.io library repo match",
				key:      "docker.io/library/busybox",
				path:     filepath.Join("testdata", "refpath.json"),
				expected: types.DockerAuthConfig{Username: "library", Password: "busybox"},
			},
			{
				name:     "docker.io library namespace match",
				key:      "docker.io/library/notbusybox",
				path:     filepath.Join("testdata", "refpath.json"),
				expected: types.DockerAuthConfig{Username: "library", Password: "other"},
			},
			{ // This tests that the docker.io/vendor key in auth file is not normalized to docker.io/library/vendor
				name:     "docker.io vendor repo match",
				key:      "docker.io/vendor/product",
				path:     filepath.Join("testdata", "refpath.json"),
				expected: types.DockerAuthConfig{Username: "first", Password: "level"},
			},
			{ // This tests that the docker.io/vendor key in the query is not normalized to docker.io/library/vendor.
				name:     "docker.io vendor namespace match",
				key:      "docker.io/vendor",
				path:     filepath.Join("testdata", "refpath.json"),
				expected: types.DockerAuthConfig{Username: "first", Password: "level"},
			},
			{
				name:     "docker.io host-only match",
				key:      "docker.io/other-vendor/other-product",
				path:     filepath.Join("testdata", "refpath.json"),
				expected: types.DockerAuthConfig{Username: "top", Password: "level"},
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if err := os.RemoveAll(configPath); err != nil {
					t.Fatal(err)
				}

				if tc.path != "" {
					contents, err := os.ReadFile(tc.path)
					if err != nil {
						t.Fatal(err)
					}

					if err := os.WriteFile(configPath, contents, 0640); err != nil {
						t.Fatal(err)
					}
				}

				var sys *types.SystemContext
				if tc.sys != nil {
					sys = tc.sys
				}

				auth, err := getCredentialsWithHomeDir(sys, tc.key, tmpHomeDir)
				require.NoError(t, err)
				assert.Equal(t, tc.expected, auth)

				// Verify the previous API also returns data consistent with the current one.
				username, password, err := getAuthenticationWithHomeDir(sys, tc.key, tmpHomeDir)
				if tc.expected.IdentityToken != "" {
					assert.Error(t, err)
				} else {
					require.NoError(t, err)
					assert.Equal(t, tc.expected.Username, username)
					assert.Equal(t, tc.expected.Password, password)
				}

				require.NoError(t, os.RemoveAll(configPath))
			})
		}
	}
}

func TestGetAuthFromLegacyFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Logf("using temporary home directory: %q", tmpDir)

	configPath := filepath.Join(tmpDir, ".dockercfg")
	contents, err := os.ReadFile(filepath.Join("testdata", "legacy.json"))
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
			if err := os.WriteFile(configPath, contents, 0640); err != nil {
				t.Fatal(err)
			}

			auth, err := getCredentialsWithHomeDir(nil, tc.hostname, tmpDir)
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
	tmpDir := t.TempDir()
	t.Logf("using temporary home directory: %q", tmpDir)

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
		contents, err := os.ReadFile(data.source)
		if err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(data.target, contents, 0640); err != nil {
			t.Fatal(err)
		}
	}

	auth, err := getCredentialsWithHomeDir(nil, "docker.io", tmpDir)
	assert.NoError(t, err)
	assert.Equal(t, "docker", auth.Username)
	assert.Equal(t, "io", auth.Password)
}

func TestGetAuthFailsOnBadInput(t *testing.T) {
	tmpXDGRuntimeDir := t.TempDir()
	t.Logf("using temporary XDG_RUNTIME_DIR directory: %q", tmpXDGRuntimeDir)
	t.Setenv("XDG_RUNTIME_DIR", tmpXDGRuntimeDir)

	tmpHomeDir := t.TempDir()
	t.Logf("using temporary home directory: %q", tmpHomeDir)

	configDir := filepath.Join(tmpXDGRuntimeDir, "containers")
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		configDir = filepath.Join(tmpHomeDir, ".config", "containers")
	}
	if err := os.MkdirAll(configDir, 0750); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "auth.json")

	// no config file present
	auth, err := getCredentialsWithHomeDir(nil, "index.docker.io", tmpHomeDir)
	if err != nil {
		t.Fatalf("got unexpected error: %#+v", err)
	}
	assert.Equal(t, types.DockerAuthConfig{}, auth)

	if err := os.WriteFile(configPath, []byte("Json rocks! Unless it doesn't."), 0640); err != nil {
		t.Fatalf("failed to write file %q: %v", configPath, err)
	}
	_, err = getCredentialsWithHomeDir(nil, "index.docker.io", tmpHomeDir)
	assert.ErrorContains(t, err, "unmarshaling JSON")

	// remove the invalid config file
	os.RemoveAll(configPath)
	// no config file present
	auth, err = getCredentialsWithHomeDir(nil, "index.docker.io", tmpHomeDir)
	if err != nil {
		t.Fatalf("got unexpected error: %#+v", err)
	}
	assert.Equal(t, types.DockerAuthConfig{}, auth)

	configPath = filepath.Join(tmpHomeDir, ".dockercfg")
	if err := os.WriteFile(configPath, []byte("I'm certainly not a json string."), 0640); err != nil {
		t.Fatalf("failed to write file %q: %v", configPath, err)
	}
	_, err = getCredentialsWithHomeDir(nil, "index.docker.io", tmpHomeDir)
	assert.ErrorContains(t, err, "unmarshaling JSON")
}

func TestGetAllCredentials(t *testing.T) {
	// Create a temporary authentication file.
	tmpFile, err := os.CreateTemp("", "auth.json.")
	require.NoError(t, err)
	authFilePath := tmpFile.Name()
	defer tmpFile.Close()
	defer os.Remove(authFilePath)
	// override PATH for executing credHelper
	path, err := os.Getwd()
	require.NoError(t, err)
	origPath := os.Getenv("PATH")
	newPath := fmt.Sprintf("%s:%s", filepath.Join(path, "testdata"), origPath)
	t.Setenv("PATH", newPath)
	t.Logf("using PATH: %q", newPath)
	err = os.Chmod(filepath.Join(path, "testdata", "docker-credential-helper-registry"), os.ModePerm)
	require.NoError(t, err)
	sys := types.SystemContext{
		AuthFilePath:                authFilePath,
		SystemRegistriesConfPath:    filepath.Join("testdata", "cred-helper-with-auth-files.conf"),
		SystemRegistriesConfDirPath: filepath.Join("testdata", "IdoNotExist"),
	}

	// Make sure that we can handle no-creds-found errors.
	t.Run("no credentials found", func(t *testing.T) {
		t.Setenv("DOCKER_CONFIG", filepath.Join(path, "testdata"))
		authConfigs, err := GetAllCredentials(nil)
		require.NoError(t, err)
		require.Empty(t, authConfigs)
	})

	for _, data := range [][]struct {
		writeKey    string
		expectedKey string
		username    string
		password    string
	}{
		{ // Basic operation, including a credential helper.
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
		{ // docker.io normalization, both namespaced and not
			{
				writeKey:    "docker.io/vendor",
				expectedKey: "docker.io/vendor",
				username:    "u1",
				password:    "p1",
			},
			{
				writeKey:    "index.docker.io", // Ideally we would even use a HTTPS URL
				expectedKey: "docker.io",
				username:    "u2",
				password:    "p2",
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
		err := os.WriteFile(authFilePath, []byte{'{', '}'}, 0700)
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

func TestAuthKeysForKey(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		expected    []string
	}{
		{
			name:  "a top-level repo",
			input: "quay.io/image",
			expected: []string{
				"quay.io/image",
				"quay.io",
			},
		},
		{
			name:  "a second-level repo",
			input: "quay.io/user/image",
			expected: []string{
				"quay.io/user/image",
				"quay.io/user",
				"quay.io",
			},
		},
		{
			name:  "a deeply-nested repo",
			input: "quay.io/a/b/c/d/image",
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
			name:  "docker.io library repo",
			input: "docker.io/library/busybox",
			expected: []string{
				"docker.io/library/busybox",
				"docker.io/library",
				"docker.io",
			},
		},
		{
			name:  "docker.io non-library repo",
			input: "docker.io/vendor/busybox",
			expected: []string{
				"docker.io/vendor/busybox",
				"docker.io/vendor",
				"docker.io",
			},
		},
	} {
		result := authKeysForKey(tc.input)
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
		tmpFile, err := os.CreateTemp("", "auth.json.set")
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
			expected := types.DockerAuthConfig{
				Username: usernamePrefix + fmt.Sprint(i),
				Password: passwordPrefix + fmt.Sprint(i),
			}
			auth, err := GetCredentials(sys, key)
			require.NoError(t, err)
			assert.Equal(t, expected, auth)
			ref, err := reference.ParseNamed(key)
			// Full-registry keys and docker.io/top-level-namespace can't be read by GetCredentialsForRef;
			// We have already tested that above, so ignore that; only verify that the two
			// return consistent results if both are possible.
			if err == nil {
				auth, err := GetCredentialsForRef(sys, ref)
				require.NoError(t, err)
				assert.Equal(t, expected, auth, ref.String())
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

		tmpFile, err := os.CreateTemp("", "auth.json")
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
	// Invalid keys
	for _, key := range []string{
		"https://my-registry.local",
		"host/foo:tag",
		"host/foo@digest",
		"localhost:5000/repo:tag",
		"localhost:5000/repo@digest",
	} {
		_, err := validateKey(key)
		assert.Error(t, err, key)
	}

	// Valid keys
	for _, tc := range []struct {
		key          string
		isNamespaced bool
	}{
		{"my-registry.local", false},
		{"my-registry.local/path", true},
		{"quay.io/a/b/c/d", true},
		{"localhost:5000", false},
		{"localhost:5000/repo", true},
	} {
		isNamespaced, err := validateKey(tc.key)
		require.NoError(t, err, tc.key)
		assert.Equal(t, tc.isNamespaced, isNamespaced, tc.key)
	}
}

func TestSetGetCredentials(t *testing.T) {
	const (
		username = "username"
		password = "password"
	)

	tmpDir := t.TempDir()

	for _, tc := range []struct {
		name            string
		set             string
		get             string
		useLegacyFormat bool
		shouldAuth      bool
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
			name:            "Should match legacy registry entry (legacy API)",
			set:             "https://quay.io/v1/",
			get:             "quay.io",
			shouldAuth:      true,
			useLegacyFormat: true,
		},
	} {

		// Create a new empty SystemContext referring an empty auth.json
		tmpFile, err := os.CreateTemp("", "auth.json-")
		require.NoError(t, err)
		defer os.RemoveAll(tmpFile.Name())

		sys := &types.SystemContext{}
		if tc.useLegacyFormat {
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
		auth, err := getCredentialsWithHomeDir(sys, tc.get, tmpDir)
		require.NoError(t, err)

		if tc.shouldAuth {
			assert.Equal(t, username, auth.Username, tc.name)
			assert.Equal(t, password, auth.Password, tc.name)
		} else {
			assert.Empty(t, auth.Username, tc.name)
			assert.Empty(t, auth.Password, tc.name)
		}
	}
}

package docker

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/containers/image/v5/types"
	digest "github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func dockerRefFromString(t *testing.T, s string) dockerReference {
	ref, err := ParseReference(s)
	require.NoError(t, err, s)
	dockerRef, ok := ref.(dockerReference)
	require.True(t, ok, s)
	return dockerRef
}

func TestSignatureStorageBaseURL(t *testing.T) {
	emptyDir := t.TempDir()
	for _, c := range []struct {
		dir, ref string
		expected string // Or "" to expect failure
	}{
		{ // Error reading configuration directory (/dev/null is not a directory)
			"/dev/null", "//busybox",
			"",
		},
		{ // No match found: expect default user storage base
			emptyDir, "//this/is/not/in/the:configuration",
			"file://" + filepath.Join(os.Getenv("HOME"), defaultUserDockerDir, "//this/is/not/in/the"),
		},
		{ // Invalid URL
			"fixtures/registries.d", "//localhost/invalid/url/test",
			"",
		},
		// URLs without a scheme: This will be rejected by consumers, so we don't really care about
		// the returned value, but it should not crash at the very least.
		{ // Absolute path
			"fixtures/registries.d", "//localhost/file/path/test",
			"/no/scheme/just/a/path/file/path/test",
		},
		{ // Relative path
			"fixtures/registries.d", "//localhost/relative/path/test",
			"no/scheme/relative/path/relative/path/test",
		},
		{ // Success
			"fixtures/registries.d", "//example.com/my/project",
			"https://lookaside.example.com/my/project",
		},
	} {
		base, err := SignatureStorageBaseURL(&types.SystemContext{RegistriesDirPath: c.dir},
			dockerRefFromString(t, c.ref), false)
		if c.expected != "" {
			require.NoError(t, err, c.ref)
			require.NotNil(t, base, c.ref)
			assert.Equal(t, c.expected, base.String(), c.ref)
		} else {
			assert.Error(t, err, c.ref)
		}
	}
}

func TestRegistriesDirPath(t *testing.T) {
	const nondefaultPath = "/this/is/not/the/default/registries.d"
	const variableReference = "$HOME"
	const rootPrefix = "/root/prefix"
	tempHome := t.TempDir()
	var userRegistriesDir = filepath.FromSlash(".config/containers/registries.d")
	userRegistriesDirPath := filepath.Join(tempHome, userRegistriesDir)
	for _, c := range []struct {
		sys             *types.SystemContext
		userFilePresent bool
		expected        string
	}{
		// The common case
		{nil, false, systemRegistriesDirPath},
		// There is a context, but it does not override the path.
		{&types.SystemContext{}, false, systemRegistriesDirPath},
		// Path overridden
		{&types.SystemContext{RegistriesDirPath: nondefaultPath}, false, nondefaultPath},
		// Root overridden
		{
			&types.SystemContext{RootForImplicitAbsolutePaths: rootPrefix},
			false,
			filepath.Join(rootPrefix, systemRegistriesDirPath),
		},
		// Root and path overrides present simultaneously,
		{
			&types.SystemContext{
				RootForImplicitAbsolutePaths: rootPrefix,
				RegistriesDirPath:            nondefaultPath,
			},
			false,
			nondefaultPath,
		},
		// User registries.d present, not overridden
		{&types.SystemContext{}, true, userRegistriesDirPath},
		// Context and user User registries.d preset simultaneously
		{&types.SystemContext{RegistriesDirPath: nondefaultPath}, true, nondefaultPath},
		// Root and user registries.d overrides present simultaneously,
		{
			&types.SystemContext{
				RootForImplicitAbsolutePaths: rootPrefix,
				RegistriesDirPath:            nondefaultPath,
			},
			true,
			nondefaultPath,
		},
		// No environment expansion happens in the overridden paths
		{&types.SystemContext{RegistriesDirPath: variableReference}, false, variableReference},
	} {
		if c.userFilePresent {
			err := os.MkdirAll(userRegistriesDirPath, 0700)
			require.NoError(t, err)
		} else {
			err := os.RemoveAll(userRegistriesDirPath)
			require.NoError(t, err)
		}
		path := registriesDirPathWithHomeDir(c.sys, tempHome)
		assert.Equal(t, c.expected, path)
	}
}

func TestLoadAndMergeConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// No registries.d exists
	config, err := loadAndMergeConfig(filepath.Join(tmpDir, "thisdoesnotexist"))
	require.NoError(t, err)
	assert.Equal(t, &registryConfiguration{Docker: map[string]registryNamespace{}}, config)

	// Empty registries.d directory
	emptyDir := filepath.Join(tmpDir, "empty")
	err = os.Mkdir(emptyDir, 0755)
	require.NoError(t, err)
	config, err = loadAndMergeConfig(emptyDir)
	require.NoError(t, err)
	assert.Equal(t, &registryConfiguration{Docker: map[string]registryNamespace{}}, config)

	// Unreadable registries.d directory
	unreadableDir := filepath.Join(tmpDir, "unreadable")
	err = os.Mkdir(unreadableDir, 0000)
	require.NoError(t, err)
	_, err = loadAndMergeConfig(unreadableDir)
	assert.Error(t, err)

	// An unreadable file in a registries.d directory
	unreadableFileDir := filepath.Join(tmpDir, "unreadableFile")
	err = os.Mkdir(unreadableFileDir, 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(unreadableFileDir, "0.yaml"), []byte("{}"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(unreadableFileDir, "1.yaml"), nil, 0000)
	require.NoError(t, err)
	_, err = loadAndMergeConfig(unreadableFileDir)
	assert.Error(t, err)

	// Invalid YAML
	invalidYAMLDir := filepath.Join(tmpDir, "invalidYAML")
	err = os.Mkdir(invalidYAMLDir, 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(invalidYAMLDir, "0.yaml"), []byte("}"), 0644)
	require.NoError(t, err)
	_, err = loadAndMergeConfig(invalidYAMLDir)
	assert.Error(t, err)

	// Duplicate DefaultDocker
	duplicateDefault := filepath.Join(tmpDir, "duplicateDefault")
	err = os.Mkdir(duplicateDefault, 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(duplicateDefault, "0.yaml"),
		[]byte("default-docker:\n lookaside: file:////tmp/something"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(duplicateDefault, "1.yaml"),
		[]byte("default-docker:\n lookaside: file:////tmp/different"), 0644)
	require.NoError(t, err)
	_, err = loadAndMergeConfig(duplicateDefault)
	assert.ErrorContains(t, err, "0.yaml")
	assert.ErrorContains(t, err, "1.yaml")

	// Duplicate DefaultDocker
	duplicateNS := filepath.Join(tmpDir, "duplicateNS")
	err = os.Mkdir(duplicateNS, 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(duplicateNS, "0.yaml"),
		[]byte("docker:\n example.com:\n  lookaside: file:////tmp/something"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(duplicateNS, "1.yaml"),
		[]byte("docker:\n example.com:\n  lookaside: file:////tmp/different"), 0644)
	require.NoError(t, err)
	_, err = loadAndMergeConfig(duplicateNS)
	assert.ErrorContains(t, err, "0.yaml")
	assert.ErrorContains(t, err, "1.yaml")

	// A fully worked example, including an empty-dictionary file and a non-.yaml file
	config, err = loadAndMergeConfig("fixtures/registries.d")
	require.NoError(t, err)
	assert.Equal(t, &registryConfiguration{
		DefaultDocker: &registryNamespace{Lookaside: "file:///mnt/companywide/signatures/for/other/repositories"},
		Docker: map[string]registryNamespace{
			"example.com":                    {Lookaside: "https://lookaside.example.com"},
			"registry.test.example.com":      {Lookaside: "http://registry.test.example.com/lookaside"},
			"registry.test.example.com:8888": {Lookaside: "http://registry.test.example.com:8889/lookaside", LookasideStaging: "https://registry.test.example.com:8889/lookaside/specialAPIserverWhichDoesNotExist"},
			"localhost":                      {Lookaside: "file:///home/mitr/mydevelopment1"},
			"localhost:8080":                 {Lookaside: "file:///home/mitr/mydevelopment2"},
			"localhost/invalid/url/test":     {Lookaside: ":emptyscheme"},
			"localhost/file/path/test":       {Lookaside: "/no/scheme/just/a/path"},
			"localhost/relative/path/test":   {Lookaside: "no/scheme/relative/path"},
			"docker.io/contoso":              {Lookaside: "https://lookaside.contoso.com/fordocker"},
			"docker.io/centos":               {Lookaside: "https://lookaside.centos.org/"},
			"docker.io/centos/mybetaproduct": {
				Lookaside:        "http://localhost:9999/mybetaWIP/lookaside",
				LookasideStaging: "file:///srv/mybetaWIP/lookaside",
			},
			"docker.io/centos/mybetaproduct:latest": {Lookaside: "https://lookaside.centos.org/"},
		},
	}, config)
}

func TestRegistryConfigurationSignatureTopLevel(t *testing.T) {
	config := registryConfiguration{
		DefaultDocker: &registryNamespace{Lookaside: "=default", LookasideStaging: "=default+w"},
		Docker:        map[string]registryNamespace{},
	}
	for _, ns := range []string{
		"localhost",
		"localhost:5000",
		"example.com",
		"example.com/ns1",
		"example.com/ns1/ns2",
		"example.com/ns1/ns2/repo",
		"example.com/ns1/ns2/repo:notlatest",
	} {
		config.Docker[ns] = registryNamespace{Lookaside: ns, LookasideStaging: ns + "+w"}
	}

	for _, c := range []struct{ input, expected string }{
		{"example.com/ns1/ns2/repo:notlatest", "example.com/ns1/ns2/repo:notlatest"},
		{"example.com/ns1/ns2/repo:unmatched", "example.com/ns1/ns2/repo"},
		{"example.com/ns1/ns2/notrepo:notlatest", "example.com/ns1/ns2"},
		{"example.com/ns1/notns2/repo:notlatest", "example.com/ns1"},
		{"example.com/notns1/ns2/repo:notlatest", "example.com"},
		{"unknown.example.com/busybox", "=default"},
		{"localhost:5000/busybox", "localhost:5000"},
		{"localhost/busybox", "localhost"},
		{"localhost:9999/busybox", "=default"},
	} {
		dr := dockerRefFromString(t, "//"+c.input)

		res := config.signatureTopLevel(dr, false)
		assert.Equal(t, c.expected, res, c.input)
		res = config.signatureTopLevel(dr, true) // test that forWriting is correctly propagated
		assert.Equal(t, c.expected+"+w", res, c.input)
	}

	config = registryConfiguration{
		Docker: map[string]registryNamespace{
			"unmatched": {Lookaside: "a", LookasideStaging: "b"},
		},
	}
	dr := dockerRefFromString(t, "//thisisnotmatched")
	res := config.signatureTopLevel(dr, false)
	assert.Equal(t, "", res)
	res = config.signatureTopLevel(dr, true)
	assert.Equal(t, "", res)
}

func TestRegistryNamespaceSignatureTopLevel(t *testing.T) {
	for _, c := range []struct {
		ns         registryNamespace
		forWriting bool
		expected   string
	}{
		{registryNamespace{LookasideStaging: "a", Lookaside: "b"}, true, "a"},
		{registryNamespace{LookasideStaging: "a", Lookaside: "b"}, false, "b"},
		{registryNamespace{Lookaside: "b"}, true, "b"},
		{registryNamespace{Lookaside: "b"}, false, "b"},
		{registryNamespace{LookasideStaging: "a"}, true, "a"},
		{registryNamespace{LookasideStaging: "a"}, false, ""},
		{registryNamespace{}, true, ""},
		{registryNamespace{}, false, ""},

		{registryNamespace{LookasideStaging: "a", Lookaside: "b", SigStoreStaging: "c", SigStore: "d"}, true, "a"},
		{registryNamespace{Lookaside: "b", SigStoreStaging: "c", SigStore: "d"}, true, "c"},
		{registryNamespace{Lookaside: "b", SigStore: "d"}, true, "b"},
		{registryNamespace{SigStore: "d"}, true, "d"},

		{registryNamespace{LookasideStaging: "a", Lookaside: "b", SigStoreStaging: "c", SigStore: "d"}, false, "b"},
		{registryNamespace{Lookaside: "b", SigStoreStaging: "c", SigStore: "d"}, false, "b"},
		{registryNamespace{Lookaside: "b", SigStore: "d"}, false, "b"},
		{registryNamespace{SigStore: "d"}, false, "d"},
	} {
		res := c.ns.signatureTopLevel(c.forWriting)
		assert.Equal(t, c.expected, res, fmt.Sprintf("%#v %v", c.ns, c.forWriting))
	}
}

func TestLookasideStorageURL(t *testing.T) {
	const mdInput = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	const mdMapped = "sha256=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	for _, c := range []struct {
		base     string
		index    int
		expected string
	}{
		{"file:///tmp", 0, "file:///tmp@" + mdMapped + "/signature-1"},
		{"file:///tmp", 1, "file:///tmp@" + mdMapped + "/signature-2"},
		{"https://localhost:5555/root", 0, "https://localhost:5555/root@" + mdMapped + "/signature-1"},
		{"https://localhost:5555/root", 1, "https://localhost:5555/root@" + mdMapped + "/signature-2"},
		{"http://localhost:5555/root", 0, "http://localhost:5555/root@" + mdMapped + "/signature-1"},
		{"http://localhost:5555/root", 1, "http://localhost:5555/root@" + mdMapped + "/signature-2"},
	} {
		baseURL, err := url.Parse(c.base)
		require.NoError(t, err)
		expectedURL, err := url.Parse(c.expected)
		require.NoError(t, err)
		res, err := lookasideStorageURL(baseURL, mdInput, c.index)
		require.NoError(t, err)
		assert.Equal(t, expectedURL, res, c.expected)
	}

	baseURL, err := url.Parse("file:///tmp")
	require.NoError(t, err)
	_, err = lookasideStorageURL(baseURL, digest.Digest("sha256:../hello"), 0)
	assert.Error(t, err)
}

func TestBuiltinDefaultLookasideStorageDir(t *testing.T) {
	base := builtinDefaultLookasideStorageDir(0)
	assert.NotNil(t, base)
	assert.Equal(t, "file://"+defaultDockerDir, base.String())

	base = builtinDefaultLookasideStorageDir(1000)
	assert.NotNil(t, base)
	assert.Equal(t, "file://"+filepath.Join(os.Getenv("HOME"), defaultUserDockerDir), base.String())
}

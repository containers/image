package sysregistriesv2

import (
	"fmt"
	"testing"

	"github.com/containers/image/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/containers/image/docker/reference"
)

var testConfig = []byte("")

func init() {
	readConf = func(_ string) ([]byte, error) {
		return testConfig, nil
	}
}

func TestParseLocation(t *testing.T) {
	var err error
	var location string

	// invalid locations
	_, err = parseLocation("https://example.com")
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "invalid location 'https://example.com': URI schemes are not supported")

	_, err = parseLocation("john.doe@example.com")
	assert.Nil(t, err)

	// valid locations
	location, err = parseLocation("example.com")
	assert.Nil(t, err)
	assert.Equal(t, "example.com", location)

	location, err = parseLocation("example.com/") // trailing slashes are stripped
	assert.Nil(t, err)
	assert.Equal(t, "example.com", location)

	location, err = parseLocation("example.com//////") // trailing slahes are stripped
	assert.Nil(t, err)
	assert.Equal(t, "example.com", location)

	location, err = parseLocation("example.com:5000/with/path")
	assert.Nil(t, err)
	assert.Equal(t, "example.com:5000/with/path", location)
}

func TestEmptyConfig(t *testing.T) {
	testConfig = []byte(``)

	configCache = make(map[string][]Registry)
	registries, err := GetRegistries(nil)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(registries))
}

func TestMirrors(t *testing.T) {
	testConfig = []byte(`
[[registry]]
location = "registry.com"

[[registry.mirror]]
location = "mirror-1.registry.com"

[[registry.mirror]]
location = "mirror-2.registry.com"
insecure = true

[[registry]]
location = "blocked.registry.com"
blocked = true`)

	configCache = make(map[string][]Registry)
	registries, err := GetRegistries(nil)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(registries))

	reg, err := FindRegistry(nil, "registry.com/image:tag")
	assert.Nil(t, err)
	assert.NotNil(t, reg)
	assert.Equal(t, 2, len(reg.Mirrors))
	assert.Equal(t, "mirror-1.registry.com", reg.Mirrors[0].Location)
	assert.False(t, reg.Mirrors[0].Insecure)
	assert.Equal(t, "mirror-2.registry.com", reg.Mirrors[1].Location)
	assert.True(t, reg.Mirrors[1].Insecure)
}

func TestMissingRegistryLocation(t *testing.T) {
	testConfig = []byte(`
[[registry]]
location = "registry-a.com"
unqualified-search = true

[[registry]]
location = "registry-b.com"

[[registry]]
unqualified-search = true`)
	configCache = make(map[string][]Registry)
	_, err := GetRegistries(nil)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "invalid location")
}

func TestMissingMirrorLocation(t *testing.T) {
	testConfig = []byte(`
[[registry]]
location = "registry-a.com"
unqualified-search = true

[[registry]]
location = "registry-b.com"
[[registry.mirror]]
location = "mirror-b.com"
[[registry.mirror]]
`)
	configCache = make(map[string][]Registry)
	_, err := GetRegistries(nil)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "invalid location")
}

func TestRefMatchesPrefix(t *testing.T) {
	for _, c := range []struct {
		ref, prefix string
		expected    bool
	}{
		// Prefix is a reference.Domain() value
		{"docker.io", "docker.io", true},
		{"docker.io", "example.com", false},
		{"example.com:5000", "example.com:5000", true},
		{"example.com:50000", "example.com:5000", false},
		{"example.com:5000", "example.com", true}, // FIXME FIXME This is unintended and undocumented, don't rely on this behavior
		{"example.com/foo", "example.com", true},
		{"example.com/foo/bar", "example.com", true},
		{"example.com/foo/bar:baz", "example.com", true},
		{"example.com/foo/bar@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "example.com", true},
		// Prefix is a reference.Named.Name() value or a repo namespace
		{"docker.io", "docker.io/library", false},
		{"docker.io/library", "docker.io/library", true},
		{"example.com/library", "docker.io/library", false},
		{"docker.io/libraryy", "docker.io/library", false},
		{"docker.io/library/busybox", "docker.io/library", true},
		{"docker.io", "docker.io/library/busybox", false},
		{"docker.io/library/busybox", "docker.io/library/busybox", true},
		{"example.com/library/busybox", "docker.io/library/busybox", false},
		{"docker.io/library/busybox2", "docker.io/library/busybox", false},
		// Prefix is a single image
		{"example.com", "example.com/foo:bar", false},
		{"example.com/foo", "example.com/foo:bar", false},
		{"example.com/foo:bar", "example.com/foo:bar", true},
		{"example.com/foo:bar2", "example.com/foo:bar", false},
		{"example.com", "example.com/foo@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false},
		{"example.com/foo", "example.com/foo@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false},
		{"example.com/foo@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "example.com/foo@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true},
		{"example.com/foo@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "example.com/foo@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false},
	} {
		res := refMatchesPrefix(c.ref, c.prefix)
		assert.Equal(t, c.expected, res, fmt.Sprintf("%s vs. %s", c.ref, c.prefix))
	}
}

func TestFindRegistry(t *testing.T) {
	testConfig = []byte(`
[[registry]]
location = "registry.com:5000"
prefix = "simple-prefix.com"

[[registry]]
location = "another-registry.com:5000"
prefix = "complex-prefix.com:4000/with/path"

[[registry]]
location = "registry.com:5000"
prefix = "another-registry.com"

[[registry]]
location = "no-prefix.com"

[[registry]]
location = "empty-prefix.com"
prefix = ""`)

	configCache = make(map[string][]Registry)
	registries, err := GetRegistries(nil)
	assert.Nil(t, err)
	assert.Equal(t, 5, len(registries))

	reg, err := FindRegistry(nil, "simple-prefix.com/foo/bar:latest")
	assert.Nil(t, err)
	assert.NotNil(t, reg)
	assert.Equal(t, "simple-prefix.com", reg.Prefix)
	assert.Equal(t, reg.Location, "registry.com:5000")

	// path match
	reg, err = FindRegistry(nil, "simple-prefix.com/")
	assert.Nil(t, err)
	assert.NotNil(t, reg)

	// hostname match
	reg, err = FindRegistry(nil, "simple-prefix.com")
	assert.Nil(t, err)
	assert.NotNil(t, reg)

	// invalid match
	reg, err = FindRegistry(nil, "simple-prefix.comx")
	assert.Nil(t, err)
	assert.Nil(t, reg)

	reg, err = FindRegistry(nil, "complex-prefix.com:4000/with/path/and/beyond:tag")
	assert.Nil(t, err)
	assert.NotNil(t, reg)
	assert.Equal(t, "complex-prefix.com:4000/with/path", reg.Prefix)
	assert.Equal(t, "another-registry.com:5000", reg.Location)

	reg, err = FindRegistry(nil, "no-prefix.com/foo:tag")
	assert.Nil(t, err)
	assert.NotNil(t, reg)
	assert.Equal(t, "no-prefix.com", reg.Prefix)
	assert.Equal(t, "no-prefix.com", reg.Location)

	reg, err = FindRegistry(nil, "empty-prefix.com/foo:tag")
	assert.Nil(t, err)
	assert.NotNil(t, reg)
	assert.Equal(t, "empty-prefix.com", reg.Prefix)
	assert.Equal(t, "empty-prefix.com", reg.Location)
}

func assertSearchRegistryLocationsEqual(t *testing.T, expected []string, regs []Registry) {
	// verify the expected registries and their order
	names := []string{}
	for _, r := range regs {
		names = append(names, r.Location)
	}
	assert.Equal(t, expected, names)
}

func TestFindUnqualifiedSearchRegistries(t *testing.T) {
	testConfig = []byte(`
[[registry]]
location = "registry-a.com"
unqualified-search = true

[[registry]]
location = "registry-b.com"

[[registry]]
location = "registry-c.com"
unqualified-search = true

[[registry]]
location = "registry-d.com"
unqualified-search = true
`)

	configCache = make(map[string][]Registry)
	registries, err := GetRegistries(nil)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(registries))

	unqRegs, err := FindUnqualifiedSearchRegistries(nil)
	assert.Nil(t, err)
	assertSearchRegistryLocationsEqual(t, []string{"registry-a.com", "registry-c.com", "registry-d.com"}, unqRegs)
}

func TestInsecureConfligs(t *testing.T) {
	testConfig = []byte(`
[[registry]]
location = "registry.com"

[[registry.mirror]]
location = "mirror-1.registry.com"

[[registry.mirror]]
location = "mirror-2.registry.com"


[[registry]]
location = "registry.com"
insecure = true
`)

	configCache = make(map[string][]Registry)
	registries, err := GetRegistries(nil)
	assert.NotNil(t, err)
	assert.Nil(t, registries)
	assert.Contains(t, err.Error(), "registry 'registry.com' is defined multiple times with conflicting 'insecure' setting")
}

func TestBlockConfligs(t *testing.T) {
	testConfig = []byte(`
[[registry]]
location = "registry.com"

[[registry.mirror]]
location = "mirror-1.registry.com"

[[registry.mirror]]
location = "mirror-2.registry.com"


[[registry]]
location = "registry.com"
blocked = true
`)

	configCache = make(map[string][]Registry)
	registries, err := GetRegistries(nil)
	assert.NotNil(t, err)
	assert.Nil(t, registries)
	assert.Contains(t, err.Error(), "registry 'registry.com' is defined multiple times with conflicting 'blocked' setting")
}

func TestUnmarshalConfig(t *testing.T) {
	testConfig = []byte(`
[[registry]]
location = "registry.com"

[[registry.mirror]]
location = "mirror-1.registry.com"

[[registry.mirror]]
location = "mirror-2.registry.com"


[[registry]]
location = "blocked.registry.com"
blocked = true


[[registry]]
location = "insecure.registry.com"
insecure = true


[[registry]]
location = "untrusted.registry.com"
insecure = true`)

	configCache = make(map[string][]Registry)
	registries, err := GetRegistries(nil)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(registries))
}

func TestV1BackwardsCompatibility(t *testing.T) {
	testConfig = []byte(`
[registries.search]
registries = ["registry-a.com////", "registry-c.com", "registry-d.com"]

[registries.block]
registries = ["registry-b.com"]

[registries.insecure]
registries = ["registry-d.com", "registry-e.com", "registry-a.com"]`)

	configCache = make(map[string][]Registry)
	registries, err := GetRegistries(nil)
	assert.Nil(t, err)
	assert.Equal(t, 5, len(registries))

	unqRegs, err := FindUnqualifiedSearchRegistries(nil)
	assert.Nil(t, err)
	assertSearchRegistryLocationsEqual(t, []string{"registry-a.com", "registry-c.com", "registry-d.com"}, unqRegs)

	// check if merging works
	reg, err := FindRegistry(nil, "registry-a.com/bar/foo/barfoo:latest")
	assert.Nil(t, err)
	assert.NotNil(t, reg)
	assert.True(t, reg.Search)
	assert.True(t, reg.Insecure)
	assert.False(t, reg.Blocked)
}

func TestMixingV1andV2(t *testing.T) {
	testConfig = []byte(`
[registries.search]
registries = ["registry-a.com", "registry-c.com"]

[registries.block]
registries = ["registry-b.com"]

[registries.insecure]
registries = ["registry-d.com", "registry-e.com", "registry-a.com"]

[[registry]]
location = "registry-a.com"
unqualified-search = true

[[registry]]
location = "registry-b.com"

[[registry]]
location = "registry-c.com"
unqualified-search = true `)

	configCache = make(map[string][]Registry)
	_, err := GetRegistries(nil)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "mixing sysregistry v1/v2 is not supported")
}

func TestConfigCache(t *testing.T) {
	testConfig = []byte(`
[[registry]]
location = "registry.com"

[[registry.mirror]]
location = "mirror-1.registry.com"

[[registry.mirror]]
location = "mirror-2.registry.com"


[[registry]]
location = "blocked.registry.com"
blocked = true


[[registry]]
location = "insecure.registry.com"
insecure = true


[[registry]]
location = "untrusted.registry.com"
insecure = true`)

	ctx := &types.SystemContext{SystemRegistriesConfPath: "foo"}

	configCache = make(map[string][]Registry)
	registries, err := GetRegistries(ctx)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(registries))

	// empty the config, but use the same SystemContext to show that the
	// previously specified registries are in the cache
	testConfig = []byte("")
	registries, err = GetRegistries(ctx)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(registries))
}

func TestInvalidateCache(t *testing.T) {
	testConfig = []byte(`
[[registry]]
location = "registry.com"

[[registry.mirror]]
location = "mirror-1.registry.com"

[[registry.mirror]]
location = "mirror-2.registry.com"


[[registry]]
location = "blocked.registry.com"
blocked = true


[[registry]]
location = "insecure.registry.com"
insecure = true


[[registry]]
location = "untrusted.registry.com"
insecure = true`)

	ctx := &types.SystemContext{}

	configCache = make(map[string][]Registry)
	registries, err := GetRegistries(ctx)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(registries))
	assertSearchRegistryLocationsEqual(t, []string{"registry.com", "blocked.registry.com", "insecure.registry.com", "untrusted.registry.com"}, registries)

	// invalidate the cache, make sure it's empty and reload
	InvalidateCache()
	assert.Equal(t, 0, len(configCache))

	registries, err = GetRegistries(ctx)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(registries))
	assertSearchRegistryLocationsEqual(t, []string{"registry.com", "blocked.registry.com", "insecure.registry.com", "untrusted.registry.com"}, registries)
}

func toNamedRef(t *testing.T, ref string) reference.Named {
	parsedRef, err := reference.ParseNamed(ref)
	require.NoError(t, err)
	return parsedRef
}

func TestRewriteReferenceSuccess(t *testing.T) {
	for _, c := range []struct{ inputRef, prefix, location, expected string }{
		// Standard use cases
		{"example.com/image", "example.com", "example.com", "example.com/image"},
		{"example.com/image:latest", "example.com", "example.com", "example.com/image:latest"},
		{"example.com:5000/image", "example.com:5000", "example.com:5000", "example.com:5000/image"},
		{"example.com:5000/image:latest", "example.com:5000", "example.com:5000", "example.com:5000/image:latest"},
		{"example.com:5000/image:latest", "", "other.com", "example.com:5000/image:latest"},
		{"example.com:5000/image:latest", "", "", "example.com:5000/image:latest"},
		// Separator test ('/', '@', ':')
		{"example.com/foo@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"example.com", "example.com",
			"example.com/foo@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{"example.com/foo/image:latest", "example.com/foo", "example.com", "example.com/image:latest"},
		{"example.com/foo/image:latest", "example.com/foo", "example.com/path", "example.com/path/image:latest"},
		// Docker examples
		{"docker.io/library/image:latest", "docker.io", "docker.io", "docker.io/library/image:latest"},
		{"docker.io/library/image", "docker.io/library", "example.com", "example.com/image"},
		{"docker.io/library/image", "docker.io", "example.com", "example.com/library/image"},
		{"docker.io/library/prefix/image", "docker.io/library/prefix", "example.com", "example.com/image"},
	} {
		ref := toNamedRef(t, c.inputRef)
		testEndpoint := Endpoint{Location: c.location}
		out, err := testEndpoint.RewriteReference(ref, c.prefix)
		require.NoError(t, err)
		assert.Equal(t, c.expected, out.String())
	}
}

func TestRewriteReferenceFailedDuringParseNamed(t *testing.T) {
	for _, c := range []struct{ inputRef, prefix, location string }{
		// Invalid reference format
		{"example.com/foo/image:latest", "example.com/foo", "example.com/path/"},
		{"example.com/foo@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"example.com/foo", "example.com"},
		{"example.com:5000/image:latest", "example.com", ""},
		{"example.com:5000/image:latest", "example.com", "example.com:5000"},
		// Malformed prefix
		{"example.com/foo/image:latest", "example.com//foo", "example.com/path"},
		{"example.com/image:latest", "image", "anotherimage"},
		{"example.com/foo/image:latest", "example.com/foo/", "example.com"},
		{"example.com/foo/image", "example.com/fo", "example.com/foo"},
		{"example.com/foo:latest", "example.com/fo", "example.com/foo"},
		{"example.com/foo@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"example.com/fo", "example.com/foo"},
		{"docker.io/library/image", "example.com", "example.com"},
	} {
		ref := toNamedRef(t, c.inputRef)
		testEndpoint := Endpoint{Location: c.location}
		out, err := testEndpoint.RewriteReference(ref, c.prefix)
		assert.NotNil(t, err)
		assert.Nil(t, out)
	}
}

func TestRewriteReferenceFailedWithNilRef(t *testing.T) {
	testEndpoint := Endpoint{}
	out, err := testEndpoint.RewriteReference(nil, "")
	assert.NotNil(t, err)
	assert.Nil(t, out)
}

package sysregistriesv2

import (
	"testing"

	"github.com/containers/image/types"
	"github.com/stretchr/testify/assert"
)

var testConfig = []byte("")

func init() {
	readConf = func(_ string) ([]byte, error) {
		return testConfig, nil
	}
}

func TestParseURL(t *testing.T) {
	var err error
	var url string

	// invalid URLs
	_, err = parseURL("https://example.com")
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "invalid URL 'https://example.com': URI schemes are not supported")

	_, err = parseURL("john.doe@example.com")
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "invalid URL 'john.doe@example.com': user/password are not supported")

	_, err = parseURL("127.0.0.1:123456")
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "invalid port number '123456' in numeric IPv4 address")

	// valid URLs
	url, err = parseURL("example.com")
	assert.Nil(t, err)
	assert.Equal(t, "example.com", url)

	url, err = parseURL("example.com/") // trailing slashes are stripped
	assert.Nil(t, err)
	assert.Equal(t, "example.com", url)

	url, err = parseURL("example.com//////") // trailing slahes are stripped
	assert.Nil(t, err)
	assert.Equal(t, "example.com", url)

	url, err = parseURL("example.com:5000/with/path")
	assert.Nil(t, err)
	assert.Equal(t, "example.com:5000/with/path", url)

	url, err = parseURL("example.com:5000")
	assert.Nil(t, err)
	assert.Equal(t, "example.com:5000", url)

	url, err = parseURL("172.30.0.1")
	assert.Nil(t, err)
	assert.Equal(t, "172.30.0.1", url)

	url, err = parseURL("172.30.0.1:5000") // often used in OpenShift
	assert.Nil(t, err)
	assert.Equal(t, "172.30.0.1:5000", url)
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
url = "registry.com"

[[registry.mirror]]
url = "mirror-1.registry.com"

[[registry.mirror]]
url = "mirror-2.registry.com"
insecure = true

[[registry]]
url = "blocked.registry.com"
blocked = true`)

	configCache = make(map[string][]Registry)
	registries, err := GetRegistries(nil)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(registries))

	var reg *Registry
	reg = FindRegistry("registry.com/image:tag", registries)
	assert.NotNil(t, reg)
	assert.Equal(t, 2, len(reg.Mirrors))
	assert.Equal(t, "mirror-1.registry.com", reg.Mirrors[0].URL)
	assert.False(t, reg.Mirrors[0].Insecure)
	assert.Equal(t, "mirror-2.registry.com", reg.Mirrors[1].URL)
	assert.True(t, reg.Mirrors[1].Insecure)
}

func TestMissingRegistryURL(t *testing.T) {
	testConfig = []byte(`
[[registry]]
url = "registry-a.com"
unqualified-search = true

[[registry]]
url = "registry-b.com"

[[registry]]
unqualified-search = true`)
	configCache = make(map[string][]Registry)
	_, err := GetRegistries(nil)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "invalid URL")
}

func TestMissingMirrorURL(t *testing.T) {
	testConfig = []byte(`
[[registry]]
url = "registry-a.com"
unqualified-search = true

[[registry]]
url = "registry-b.com"
[[registry.mirror]]
url = "mirror-b.com"
[[registry.mirror]]
`)
	configCache = make(map[string][]Registry)
	_, err := GetRegistries(nil)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "invalid URL")
}

func TestFindRegistry(t *testing.T) {
	testConfig = []byte(`
[[registry]]
url = "registry.com:5000"
prefix = "simple-prefix.com"

[[registry]]
url = "another-registry.com:5000"
prefix = "complex-prefix.com:4000/with/path"

[[registry]]
url = "registry.com:5000"
prefix = "another-registry.com"

[[registry]]
url = "no-prefix.com"

[[registry]]
url = "empty-prefix.com"
prefix = ""`)

	configCache = make(map[string][]Registry)
	registries, err := GetRegistries(nil)
	assert.Nil(t, err)
	assert.Equal(t, 5, len(registries))

	var reg *Registry
	reg = FindRegistry("simple-prefix.com/foo/bar:latest", registries)
	assert.NotNil(t, reg)
	assert.Equal(t, "simple-prefix.com", reg.Prefix)
	assert.Equal(t, reg.URL, "registry.com:5000")

	reg = FindRegistry("complex-prefix.com:4000/with/path/and/beyond:tag", registries)
	assert.NotNil(t, reg)
	assert.Equal(t, "complex-prefix.com:4000/with/path", reg.Prefix)
	assert.Equal(t, "another-registry.com:5000", reg.URL)

	reg = FindRegistry("no-prefix.com/foo:tag", registries)
	assert.NotNil(t, reg)
	assert.Equal(t, "no-prefix.com", reg.Prefix)
	assert.Equal(t, "no-prefix.com", reg.URL)

	reg = FindRegistry("empty-prefix.com/foo:tag", registries)
	assert.NotNil(t, reg)
	assert.Equal(t, "empty-prefix.com", reg.Prefix)
	assert.Equal(t, "empty-prefix.com", reg.URL)
}

func assertSearchRegistryURLsEqual(t *testing.T, expected []string, regs []Registry) {
	// verify the expected registries and their order
	names := []string{}
	for _, r := range regs {
		names = append(names, r.URL)
	}
	assert.Equal(t, expected, names)
}

func TestFindUnqualifiedSearchRegistries(t *testing.T) {
	testConfig = []byte(`
[[registry]]
url = "registry-a.com"
unqualified-search = true

[[registry]]
url = "registry-b.com"

[[registry]]
url = "registry-c.com"
unqualified-search = true

[[registry]]
url = "registry-d.com"
unqualified-search = true
`)

	configCache = make(map[string][]Registry)
	registries, err := GetRegistries(nil)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(registries))

	unqRegs := FindUnqualifiedSearchRegistries(registries)
	assertSearchRegistryURLsEqual(t, []string{"registry-a.com", "registry-c.com", "registry-d.com"}, unqRegs)
}

func TestInsecureConfligs(t *testing.T) {
	testConfig = []byte(`
[[registry]]
url = "registry.com"

[[registry.mirror]]
url = "mirror-1.registry.com"

[[registry.mirror]]
url = "mirror-2.registry.com"


[[registry]]
url = "registry.com"
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
url = "registry.com"

[[registry.mirror]]
url = "mirror-1.registry.com"

[[registry.mirror]]
url = "mirror-2.registry.com"


[[registry]]
url = "registry.com"
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
url = "registry.com"

[[registry.mirror]]
url = "mirror-1.registry.com"

[[registry.mirror]]
url = "mirror-2.registry.com"


[[registry]]
url = "blocked.registry.com"
blocked = true


[[registry]]
url = "insecure.registry.com"
insecure = true


[[registry]]
url = "untrusted.registry.com"
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

	unqRegs := FindUnqualifiedSearchRegistries(registries)
	assertSearchRegistryURLsEqual(t, []string{"registry-a.com", "registry-c.com", "registry-d.com"}, unqRegs)

	// check if merging works
	reg := FindRegistry("registry-a.com/bar/foo/barfoo:latest", registries)
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
url = "registry-a.com"
unqualified-search = true

[[registry]]
url = "registry-b.com"

[[registry]]
url = "registry-c.com"
unqualified-search = true `)

	configCache = make(map[string][]Registry)
	_, err := GetRegistries(nil)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "mixing sysregistry v1/v2 is not supported")
}

func TestConfigCache(t *testing.T) {
	testConfig = []byte(`
[[registry]]
url = "registry.com"

[[registry.mirror]]
url = "mirror-1.registry.com"

[[registry.mirror]]
url = "mirror-2.registry.com"


[[registry]]
url = "blocked.registry.com"
blocked = true


[[registry]]
url = "insecure.registry.com"
insecure = true


[[registry]]
url = "untrusted.registry.com"
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

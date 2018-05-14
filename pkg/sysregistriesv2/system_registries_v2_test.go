package sysregistriesv2

import (
	"github.com/containers/image/types"
	"github.com/stretchr/testify/assert"
	"net/url"
	"testing"
)

var testConfig = []byte("")

func init() {
	readConf = func(_ *types.SystemContext) ([]byte, error) {
		return testConfig, nil
	}
}

func TestParseURL(t *testing.T) {
	var err error
	var url url.URL

	// invalid URLs
	_, err = parseURL("unspecified.scheme")
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "unspecified URI scheme:")

	_, err = parseURL("httpx://unsupported.scheme")
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "unsupported URI scheme:")

	_, err = parseURL("http://")
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "unspecified URI host:")

	_, err = parseURL("https://user:password@unsupported.com")
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "unsupported username/password:")

	// valid URLs
	url, err = parseURL("http://example.com")
	assert.Nil(t, err)
	assert.Equal(t, "http://example.com", url.String())

	url, err = parseURL("http://example.com/")
	assert.Nil(t, err)
	assert.Equal(t, "http://example.com", url.String())

	url, err = parseURL("http://example.com//////")
	assert.Nil(t, err)
	assert.Equal(t, "http://example.com", url.String())

	url, err = parseURL("http://example.com:5000/with/path")
	assert.Nil(t, err)
	assert.Equal(t, "http://example.com:5000/with/path", url.String())
}

func TestEmptyConfig(t *testing.T) {
	testConfig = []byte(``)

	registries, err := GetRegistries(nil)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(registries))
}

func TestMirrors(t *testing.T) {
	testConfig = []byte(`
[[registry]]
url = "https://registry.com"

[[registry.mirror]]
url = "https://mirror-1.registry.com"

[[registry.mirror]]
url = "https://mirror-2.registry.com"
insecure = true

[[registry]]
url = "https://blocked.registry.com"
blocked = true`)

	registries, err := GetRegistries(nil)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(registries))

	var reg *Registry
	reg = FindRegistry("registry.com/image:tag", registries)
	assert.NotNil(t, reg)
	assert.Equal(t, 2, len(reg.Mirrors))
	assert.Equal(t, "https://mirror-1.registry.com", reg.Mirrors[0].URL.String())
	assert.False(t, reg.Mirrors[0].Insecure)
	assert.Equal(t, "https://mirror-2.registry.com", reg.Mirrors[1].URL.String())
	assert.True(t, reg.Mirrors[1].Insecure)
}

func TestMissingRegistryURL(t *testing.T) {
	testConfig = []byte(`
[[registry]]
url = "https://registry-a.com"
unqualified-search = true

[[registry]]
url = "https://registry-b.com"

[[registry]]
unqualified-search = true`)
	_, err := GetRegistries(nil)
	assert.NotNil(t, err)
	assert.Contains(t, "registry must include a URL", err.Error())
}

func TestMissingMirrorURL(t *testing.T) {
	testConfig = []byte(`
[[registry]]
url = "https://registry-a.com"
unqualified-search = true

[[registry]]
url = "https://registry-b.com"
[[registry.mirror]]
url = "https://mirror-b.com"
[[registry.mirror]]
`)
	_, err := GetRegistries(nil)
	assert.NotNil(t, err)
	assert.Contains(t, "mirror must include a URL", err.Error())
}

func TestFindRegistry(t *testing.T) {
	testConfig = []byte(`
[[registry]]
url = "https://registry.com:5000"
prefix = "simple-prefix.com"

[[registry]]
url = "https://another-registry.com:5000"
prefix = "complex-prefix.com:4000/with/path"

[[registry]]
url = "https://registry.com:5000"
prefix = "another-registry.com"

[[registry]]
url = "https://no-prefix.com"

[[registry]]
url = "https://empty-prefix.com"
prefix = ""`)

	registries, err := GetRegistries(nil)
	assert.Nil(t, err)
	assert.Equal(t, 5, len(registries))

	var reg *Registry
	reg = FindRegistry("simple-prefix.com/foo/bar:latest", registries)
	assert.NotNil(t, reg)
	assert.Equal(t, "simple-prefix.com", reg.Prefix)
	assert.Equal(t, reg.URL.String(), "https://registry.com:5000")

	reg = FindRegistry("complex-prefix.com:4000/with/path/and/beyond:tag", registries)
	assert.NotNil(t, reg)
	assert.Equal(t, "complex-prefix.com:4000/with/path", reg.Prefix)
	assert.Equal(t, "https://another-registry.com:5000", reg.URL.String())

	reg = FindRegistry("no-prefix.com/foo:tag", registries)
	assert.NotNil(t, reg)
	assert.Equal(t, "no-prefix.com", reg.Prefix)
	assert.Equal(t, "https://no-prefix.com", reg.URL.String())

	reg = FindRegistry("empty-prefix.com/foo:tag", registries)
	assert.NotNil(t, reg)
	assert.Equal(t, "empty-prefix.com", reg.Prefix)
	assert.Equal(t, "https://empty-prefix.com", reg.URL.String())
}

func TestFindUnqualifiedSearchRegistries(t *testing.T) {
	testConfig = []byte(`
[[registry]]
url = "https://registry-a.com"
unqualified-search = true

[[registry]]
url = "https://registry-b.com"

[[registry]]
url = "https://registry-c.com"
unqualified-search = true`)

	registries, err := GetRegistries(nil)
	assert.Nil(t, err)
	assert.Equal(t, 3, len(registries))

	unqRegs := FindUnqualifiedSearchRegistries(registries)
	assert.Equal(t, 2, len(unqRegs))

	// check if the expected images are actually in the array
	var reg *Registry
	reg = FindRegistry("registry-a.com/foo:bar", unqRegs)
	assert.NotNil(t, reg)
	reg = FindRegistry("registry-c.com/foo:bar", unqRegs)
	assert.NotNil(t, reg)
}

func TestUnmarshalConfig(t *testing.T) {
	testConfig = []byte(`
[[registry]]
url = "https://registry.com"

[[registry.mirror]]
url = "https://mirror-1.registry.com"

[[registry.mirror]]
url = "https://mirror-2.registry.com"


[[registry]]
url = "https://blocked.registry.com"
blocked = true


[[registry]]
url = "http://insecure.registry.com"
insecure = true


[[registry]]
url = "https://untrusted.registry.com"
insecure = true`)

	registries, err := GetRegistries(nil)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(registries))
}

func TestV1BackwardsCompatibility(t *testing.T) {
	testConfig = []byte(`
[registries.search]
registries = ["registry-a.com////", "https://registry-c.com"]

[registries.block]
registries = ["https://registry-b.com"]

[registries.insecure]
registries = ["https://registry-d.com", "https://registry-e.com", "https://registry-a.com"]`)

	registries, err := GetRegistries(nil)
	assert.Nil(t, err)
	assert.Equal(t, 5, len(registries))

	unqRegs := FindUnqualifiedSearchRegistries(registries)
	assert.Equal(t, 2, len(unqRegs))

	// check if the expected images are actually in the array
	var reg *Registry
	reg = FindRegistry("registry-a.com/foo:bar", unqRegs)
	// test https fallback for v1
	assert.Equal(t, "https://registry-a.com", reg.URL.String())
	assert.NotNil(t, reg)
	reg = FindRegistry("registry-c.com/foo:bar", unqRegs)
	assert.NotNil(t, reg)

	// check if merging works
	reg = FindRegistry("registry-a.com/bar/foo/barfoo:latest", registries)
	assert.NotNil(t, reg)
	assert.True(t, reg.Search)
	assert.True(t, reg.Insecure)
	assert.False(t, reg.Blocked)
}

func TestMixingV1andV2(t *testing.T) {
	testConfig = []byte(`
[registries.search]
registries = ["https://registry-a.com", "https://registry-c.com"]

[registries.block]
registries = ["https://registry-b.com"]

[registries.insecure]
registries = ["https://registry-d.com", "https://registry-e.com", "https://registry-a.com"]

[[registry]]
url = "https://registry-a.com"
unqualified-search = true

[[registry]]
url = "https://registry-b.com"

[[registry]]
url = "https://registry-c.com"
unqualified-search = true `)

	_, err := GetRegistries(nil)
	assert.NotNil(t, err)
	assert.Contains(t, "mixing sysregistry v1/v2 is not supported", err.Error())
}

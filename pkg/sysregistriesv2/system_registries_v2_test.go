package sysregistriesv2

import (
	"fmt"
	"github.com/containers/image/types"
	"github.com/stretchr/testify/assert"
	"testing"
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

func TestFindUnqualifiedSearchRegistries(t *testing.T) {
	testConfig = []byte(`
[[registry]]
url = "registry-a.com"
unqualified-search = true

[[registry]]
url = "registry-b.com"

[[registry]]
url = "registry-c.com"
unqualified-search = true`)

	configCache = make(map[string][]Registry)
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

func TestInsecureConflicts(t *testing.T) {
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
registries = ["registry-a.com////", "registry-c.com"]

[registries.block]
registries = ["registry-b.com"]

[registries.insecure]
registries = ["registry-d.com", "registry-e.com", "registry-a.com"]`)

	configCache = make(map[string][]Registry)
	registries, err := GetRegistries(nil)
	assert.Nil(t, err)
	assert.Equal(t, 5, len(registries))

	unqRegs := FindUnqualifiedSearchRegistries(registries)
	assert.Equal(t, 2, len(unqRegs))

	// check if the expected images are actually in the array
	var reg *Registry
	reg = FindRegistry("registry-a.com/foo:bar", unqRegs)
	// test https fallback for v1
	assert.Equal(t, "registry-a.com", reg.URL)
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

func TestParseMatchPattern(t *testing.T) {
	cases := []struct {
		pattern, host, port, path string
		err                       bool
	}{
		{"localhost", "localhost", "", "", false},
		{"localhost:5000", "localhost", "5000", "", false},
		{"localhost/foo", "localhost", "", "/foo", false},
		{"localhost:5000/foo", "localhost", "5000", "/foo", false},

		{"[172.31.0.1]", "172.31.0.1", "", "", false},
		{"[172.31.0.1000]", "", "", "", true},
		{"[::]", "::", "", "", false},
		{"[::1ffff]", "", "", "", true},
		{"[::12345]", "", "", "", true},

		{"172.31.0.1", "172.31.0.1", "", "", false},
		{"172.31.0.1/foo", "172.31.0.1", "", "/foo", false},
		{"172.31.0.1:5000", "172.31.0.1", "5000", "", false},
		{"172.31.0.1:5000/foo", "172.31.0.1", "5000", "/foo", false},
		{"172.31.0.1:50000/foo", "172.31.0.1", "50000", "/foo", false},

		{"172.31.0.1000:50000/foo", "172.31.0.1000", "50000", "/foo", true},
		{"172.31.0.1:500000/foo", "", "", "", true},

		{"172.31.0.1/23", "172.31.0.1/23", "", "", false},
		{"172.31.0.1/23/foo", "172.31.0.1/23", "", "/foo", false},
		{"172.31.0.1/23:5000", "172.31.0.1/23", "5000", "", false},
		{"172.31.0.1/23:5000/foo", "172.31.0.1/23", "5000", "/foo", false},

		{"[::1/64]", "::1/64", "", "", false},
		{"[::1/64]/foo", "::1/64", "", "/foo", false},
		{"[::1/64]:5000", "::1/64", "5000", "", false},
		{"[::1/64]:5000/foo", "::1/64", "5000", "/foo", false},

		{"172.31.0.*", "172.31.0.*", "", "", false},
		{"172.31.0.*/foo", "172.31.0.*", "", "/foo", false},
		{"172.31.0.*:5000", "172.31.0.*", "5000", "", false},
		{"172.31.0.*:5000/foo", "172.31.0.*", "5000", "/foo", false},
	}
	for _, c := range cases {
		host, port, path, err := parseMatchPattern(c.pattern)
		t.Logf("%+v -> %q,%q,%q,%v", c, host, port, path, err)
		if c.err {
			assert.NotNil(t, err)
		} else {
			assert.Nil(t, err)
			assert.Equal(t, c.host, host)
			assert.Equal(t, c.port, port)
			assert.Equal(t, c.path, path)
		}
	}
}

func TestParseMatchCandidate(t *testing.T) {
	cases := []struct {
		candidate, host, port, path string
		err                         bool
	}{
		{"localhost", "localhost", "", "", false},
		{"localhost:5000", "localhost", "5000", "", false},
		{"localhost/foo", "localhost", "", "/foo", false},
		{"localhost:5000/foo", "localhost", "5000", "/foo", false},

		{"172.31.0.1", "172.31.0.1", "", "", false},
		{"172.31.0.1/foo", "172.31.0.1", "", "/foo", false},
		{"172.31.0.1:5000", "172.31.0.1", "5000", "", false},
		{"172.31.0.1:5000/foo", "172.31.0.1", "5000", "/foo", false},
		{"172.31.0.1:50000/foo", "172.31.0.1", "50000", "/foo", false},

		{"172.31.0.1000:50000/foo", "172.31.0.1000", "50000", "/foo", false},
		{"172.31.0.1:500000/foo", "", "", "", true},

		{"172.31.0.1/23", "172.31.0.1", "", "/23", false},
		{"172.31.0.1/23/foo", "172.31.0.1", "", "/23/foo", false},
		{"172.31.0.1/23:5000", "172.31.0.1", "", "/23:5000", false},
		{"172.31.0.1/23:5000/foo", "172.31.0.1", "", "/23:5000/foo", false},

		{"[::1]", "::1", "", "", false},
		{"[::1]/foo", "::1", "", "/foo", false},
		{"[::1]:5000", "::1", "5000", "", false},
		{"[::1]:5000/foo", "::1", "5000", "/foo", false},

		{"[::1/64]", "", "", "", true},
		{"[::1/64]/foo", "", "", "", true},
		{"[::1/64]:5000", "", "", "", true},
		{"[::1/64]:5000/foo", "", "", "", true},

		{"172.31.0.*", "172.31.0.*", "", "", false},
		{"172.31.0.*/foo", "172.31.0.*", "", "/foo", false},
		{"172.31.0.*:5000", "172.31.0.*", "5000", "", false},
		{"172.31.0.*:5000/foo", "172.31.0.*", "5000", "/foo", false},
	}
	for _, c := range cases {
		host, port, path, err := parseMatchCandidate(c.candidate)
		t.Logf("%+v -> %q,%q,%q,%v", c, host, port, path, err)
		if c.err {
			assert.NotNil(t, err)
		} else {
			assert.Nil(t, err)
			assert.Equal(t, c.host, host)
			assert.Equal(t, c.port, port)
			assert.Equal(t, c.path, path)
		}
	}
}

func TestRegistryMatch(t *testing.T) {
	cases := []struct {
		pattern, candidate string
		match, err         bool
	}{
		{"localhost", "localhost", true, false},
		{"*", "localhost", true, false},
		{"*", "localhost:500", false, false},
		{"*", "localhost:5000", true, false},

		{"*", "registry.example.com", true, false},
		{"*.example.com", "registry.example.com", true, false},
		{"*.example.com", "registry.example.com:5000", true, false},
		{"*.example.com", "registry.example.com:5000/foo", true, false},
		{"*.example.com", "registry.example.com/foo", true, false},
		{"*.example.com", "*", false, false},
		{"*.example.com", "registry.example.org", false, false},
		{"*.example.com", "example.com.example.org", false, false},

		{"*", "172.31.0.1", true, false},
		{"*", "172.31.0.1:443", false, false},
		{"*", "172.31.0.1:5000", true, false},
		{"*", "172.31.0.1/foo", true, false},
		{"*", "172.31.0.1:443/foo", false, false},
		{"*", "172.31.0.1:5000/foo", true, false},

		{"172.31.0.*", "172.31.0.1", true, false},
		{"172.31.0.*", "172.31.0.1:443", false, false},
		{"172.31.0.*", "172.31.0.1:5000", true, false},
		{"172.31.0.*", "172.31.0.1/foo", true, false},
		{"172.31.0.*", "172.31.0.1:443/foo", false, false},
		{"172.31.0.*", "172.31.0.1:5000/foo", true, false},

		{"172.31.0.*", "172.31.1.1", false, false},
		{"172.31.0.*", "172.31.1.1:5000", false, false},
		{"172.31.0.*", "172.31.1.1/foo", false, false},
		{"172.31.0.*", "172.31.1.1:5000/foo", false, false},

		{"172.31.0.0/33", "172.31.0.1/foo", false, false},
		{"172.31.0.0/24", "172.31.0.1", true, false},
		{"172.31.0.0/24", "172.31.0.1/foo", true, false},
		{"172.31.0.0/24/foo", "172.31.0.1/foo", true, false},
		{"172.31.0.0/24:5000/foo", "172.31.0.1/foo", true, false},
		{"172.31.0.0/24/foo", "172.31.0.1:5000/foo", true, false},
		{"172.31.0.0/24/foo", "172.31.0.1", false, false},

		{"172.31.0.0/33", "172.31.0.1/foo", false, false},
		{"172.31.0.0/24", "172.31.0.1", true, false},
		{"172.31.0.0/24", "172.31.0.1/foo", true, false},
		{"172.31.0.0/24/foo", "172.31.0.1/foo", true, false},
		{"172.31.0.0/24:5000/foo", "172.31.0.1/foo", true, false},
		{"172.31.0.0/24/foo", "172.31.0.1:5000/foo", true, false},
		{"172.31.0.0/24/foo", "172.31.0.1", false, false},
	}
	for _, c := range cases {
		matches, err := registryMatches(c.pattern, c.candidate)
		t.Logf("%+v -> %v,%v", c, matches, err)
		if c.err {
			assert.NotNil(t, err)
		} else {
			assert.Equal(t, c.match, matches)
		}
	}
}

func TestMatchV2(t *testing.T) {
	testConfig = []byte(`
[[registry]]
url = "registry.com"
insecure = true

[[registry]]
url = "registry.org"

[[registry]]
prefix = "registry.net/images"
insecure = true
`)

	configCache = make(map[string][]Registry)
	registries, err := GetRegistries(nil)
	assert.Nil(t, err)
	assert.Equal(t, 3, len(registries))

	var reg *Registry
	reg = FindRegistry("registry.com/image:tag", registries)
	assert.NotNil(t, reg)
	assert.True(t, reg.Insecure)

	reg = FindRegistry("registry.org/image", registries)
	assert.NotNil(t, reg)
	assert.False(t, reg.Insecure)

	reg = FindRegistry("registry.net/images/foo", registries)
	assert.NotNil(t, reg)
	assert.True(t, reg.Insecure)

	reg = FindRegistry("registry.net/image/foo", registries)
	assert.Nil(t, reg)

	reg = FindRegistry("registry.foo", registries)
	assert.Nil(t, reg)
}

func TestMatchV1(t *testing.T) {
	var reg *Registry
	for _, c := range []struct {
		pattern, candidate string
		insecure           bool
	}{
		{"example.com", "example.org", false},
		{"example.com", "example.org:5000", false},
		{"example.com", "example.org/foo", false},
		{"example.com", "example.org:5000/foo", false},

		{"example.com", "example.com", true},
		{"example.com", "example.com:5000", true},
		{"example.com", "example.com/foo", true},
		{"example.com", "example.com:5000/foo", true},

		{"*.example.com", "example.com", false},
		{"*.example.com", "example.com:5000", false},
		{"*.example.com", "example.com/foo", false},
		{"*.example.com", "example.com:5000/foo", false},

		{"*.example.com", "registry.example.com", true},
		{"*.example.com", "registry.example.com:5000", true},
		{"*.example.com", "registry.example.com/foo", true},
		{"*.example.com", "registry.example.com:5000/foo", true},

		{"*.example.com/foo", "registry.example.com", false},
		{"*.example.com/foo", "registry.example.com:5000", false},
		{"*.example.com/foo", "registry.example.com/foo", true},
		{"*.example.com/foo", "registry.example.com:5000/foo", true},

		{"127.0.0.1/8/foo", "127.0.0.2", false},
		{"127.0.0.1/8/foo", "127.0.0.2:5000", false},
		{"127.0.0.1/8/foo", "127.0.0.2/foo", true},
		{"127.0.0.1/8/foo", "127.0.0.2:5000/foo", true},

		{"127.0.0.1/8:5000/foo", "127.0.0.2", false},
		{"127.0.0.1/8:5000/foo", "127.0.0.2:5000", false},
		{"127.0.0.1/8:5000/foo", "127.0.0.2/foo", true},
		{"127.0.0.1/8:5000/foo", "127.0.0.2:5000/foo", true},

		{"127.0.0.1/8:443/foo", "127.0.0.2", false},
		{"127.0.0.1/8:443/foo", "127.0.0.2:5000", false},
		{"127.0.0.1/8:443/foo", "127.0.0.2/foo", false},
		{"127.0.0.1/8:443/foo", "127.0.0.2:5000/foo", false},

		{"127.0.0.*/foo", "127.0.0.2", false},
		{"127.0.0.*/foo", "127.0.0.2:5000", false},
		{"127.0.0.*/foo", "127.0.0.2/foo", true},
		{"127.0.0.*/foo", "127.0.0.2:5000/foo", true},

		{"127.0.0.*:5000/foo", "127.0.0.2", false},
		{"127.0.0.*:5000/foo", "127.0.0.2:5000", false},
		{"127.0.0.*:5000/foo", "127.0.0.2/foo", true},
		{"127.0.0.*:5000/foo", "127.0.0.2:5000/foo", true},

		{"127.0.0.*:443/foo", "127.0.0.2", false},
		{"127.0.0.*:443/foo", "127.0.0.2:5000", false},
		{"127.0.0.*:443/foo", "127.0.0.2/foo", false},
		{"127.0.0.*:443/foo", "127.0.0.2:5000/foo", false},
	} {
		testConfig = []byte(fmt.Sprintf("[registries.insecure]\nregistries = ['%s']\n", c.pattern))
		configCache = make(map[string][]Registry)
		registries, err := GetRegistries(nil)
		assert.Nil(t, err)
		assert.Equal(t, 1, len(registries))
		reg = FindRegistry(c.candidate, registries)
		if c.insecure {
			assert.NotNil(t, reg)
			assert.True(t, reg.Insecure)
		} else {
			assert.Nil(t, reg)
		}
	}
}

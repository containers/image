package sysregistriesv2

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	location, err = parseLocation("example.com//////") // trailing slashes are stripped
	assert.Nil(t, err)
	assert.Equal(t, "example.com", location)

	location, err = parseLocation("example.com:5000/with/path")
	assert.Nil(t, err)
	assert.Equal(t, "example.com:5000/with/path", location)
}

func TestEmptyConfig(t *testing.T) {
	registries, err := GetRegistries(&types.SystemContext{
		SystemRegistriesConfPath:    "testdata/empty.conf",
		SystemRegistriesConfDirPath: "testdata/this-does-not-exist",
	})
	assert.Nil(t, err)
	assert.Equal(t, 0, len(registries))

	// When SystemRegistriesConfPath is not explicitly specified (but RootForImplicitAbsolutePaths might be), missing file is treated
	// the same as an empty one, without reporting an error.
	nonexistentRoot, err := filepath.Abs("testdata/this-does-not-exist")
	require.NoError(t, err)
	registries, err = GetRegistries(&types.SystemContext{
		RootForImplicitAbsolutePaths: nonexistentRoot,
		SystemRegistriesConfDirPath:  "testdata/this-does-not-exist",
	})
	assert.Nil(t, err)
	assert.Equal(t, 0, len(registries))
}

func TestMirrors(t *testing.T) {
	sys := &types.SystemContext{
		SystemRegistriesConfPath:    "testdata/mirrors.conf",
		SystemRegistriesConfDirPath: "testdata/this-does-not-exist",
	}

	registries, err := GetRegistries(sys)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(registries))

	reg, err := FindRegistry(sys, "registry.com/image:tag")
	assert.Nil(t, err)
	assert.NotNil(t, reg)
	assert.Equal(t, 2, len(reg.Mirrors))
	assert.Equal(t, "mirror-1.registry.com", reg.Mirrors[0].Location)
	assert.False(t, reg.Mirrors[0].Insecure)
	assert.Equal(t, "mirror-2.registry.com", reg.Mirrors[1].Location)
	assert.True(t, reg.Mirrors[1].Insecure)
}

func TestRefMatchingSubdomainPrefix(t *testing.T) {
	for _, c := range []struct {
		ref, prefix string
		expected    int
	}{
		// Check for subdomain matches
		{"docker.io", "*.io", len("docker.io")},
		{"docker.io/foo", "*.com", -1},
		{"example.com/foo", "*.co", -1},
		{"example.com/foo", "*.example.com", -1},
		//FIXME: Port Number matching needs to be revisited.
		// https://github.com/containers/image/pull/1191#pullrequestreview-631869416
		//{"example.com:5000", "*.com", len("example.com")},
		//{"example.com:5000/foo", "*.com", len("example.com")},
		//{"sub.example.com:5000/foo", "*.example.com", len("sub.example.com")},
		//{"example.com:5000/foo/bar", "*.com", len("example.com")},
		//{"example.com:5000/foo/bar:baz", "*.com", len("example.com")},
		//{"example.com:5000/foo/bar/bbq:baz", "*.com", len("example.com")},
		//{"example.com:50000/foo", "*.example.com", -1},
		{"example.com/foo", "*.com", len("example.com")},
		{"example.com/foo:bar", "*.com", len("example.com")},
		{"example.com/foo/bar:baz", "*.com", len("example.com")},
		{"yet.another.example.com/foo", "**.example.com", -1},
		{"yet.another.example.com/foo", "***.another.example.com", -1},
		{"yet.another.example.com/foo", "**********.another.example.com", -1},
		{"yet.another.example.com/foo/bar", "**********.another.example.com", -1},
		{"yet.another.example.com/foo/bar", "*.another.example.com", len("yet.another.example.com")},
		{"another.example.com/namespace.com/foo/bar/bbq:baz", "*.example.com", len("another.example.com")},
		{"example.net/namespace-ends-in.com/foo/bar/bbq:baz", "*.com", -1},
		{"another.example.com/namespace.com/foo/bar/bbq:baz", "*.namespace.com", -1},
		{"sub.example.com/foo/bar", "*.com", len("sub.example.com")},
		{"sub.example.com/foo/bar", "*.example.com", len("sub.example.com")},
		{"another.sub.example.com/foo/bar/bbq:baz", "*.example.com", len("another.sub.example.com")},
		{"another.sub.example.com/foo/bar/bbq:baz", "*.sub.example.com", len("another.sub.example.com")},
		{"yet.another.example.com/foo/bar@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "*.example.com", len("yet.another.example.com")},
		{"yet.another.sub.example.com/foo/bar@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "*.sub.example.com", len("yet.another.sub.example.com")},
	} {
		refLen := refMatchingSubdomainPrefix(c.ref, c.prefix)
		assert.Equal(t, c.expected, refLen, fmt.Sprintf("%s vs. %s", c.ref, c.prefix))
	}
}

func TestRefMatchingPrefix(t *testing.T) {
	for _, c := range []struct {
		ref, prefix string
		expected    int
	}{
		// Prefix is a reference.Domain() value
		{"docker.io", "docker.io", len("docker.io")},
		{"docker.io", "example.com", -1},
		{"example.com:5000", "example.com:5000", len("example.com:5000")},
		{"example.com:50000", "example.com:5000", -1},
		{"example.com:5000", "example.com", len("example.com")}, // FIXME FIXME This is unintended and undocumented, don't rely on this behavior
		{"example.com/foo", "example.com", len("example.com")},
		{"example.com/foo/bar", "example.com", len("example.com")},
		{"example.com/foo/bar:baz", "example.com", len("example.com")},
		{"example.com/foo/bar@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "example.com", len("example.com")},
		// Prefix is a reference.Named.Name() value or a repo namespace
		{"docker.io", "docker.io/library", -1},
		{"docker.io/library", "docker.io/library", len("docker.io/library")},
		{"example.com/library", "docker.io/library", -1},
		{"docker.io/libraryy", "docker.io/library", -1},
		{"docker.io/library/busybox", "docker.io/library", len("docker.io/library")},
		{"docker.io", "docker.io/library/busybox", -1},
		{"docker.io/library/busybox", "docker.io/library/busybox", len("docker.io/library/busybox")},
		{"example.com/library/busybox", "docker.io/library/busybox", -1},
		{"docker.io/library/busybox2", "docker.io/library/busybox", -1},
		// Prefix is a single image
		{"example.com", "example.com/foo:bar", -1},
		{"example.com/foo", "example.com/foo:bar", -1},
		{"example.com/foo:bar", "example.com/foo:bar", len("example.com/foo:bar")},
		{"example.com/foo:bar2", "example.com/foo:bar", -1},
		{"example.com", "example.com/foo@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", -1},
		{"example.com/foo", "example.com/foo@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", -1},
		{"example.com/foo@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "example.com/foo@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			len("example.com/foo@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")},
		{"example.com/foo@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "example.com/foo@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", -1},
	} {
		prefixLen := refMatchingPrefix(c.ref, c.prefix)
		assert.Equal(t, c.expected, prefixLen, fmt.Sprintf("%s vs. %s", c.ref, c.prefix))
	}
}

func TestNewConfigWrapper(t *testing.T) {
	const nondefaultPath = "/this/is/not/the/default/registries.conf"
	const variableReference = "$HOME"
	const rootPrefix = "/root/prefix"
	tempHome, err := ioutil.TempDir("", "tempHome")
	require.NoError(t, err)
	defer os.RemoveAll(tempHome)
	var userRegistriesFile = filepath.FromSlash(".config/containers/registries.conf")
	userRegistriesFilePath := filepath.Join(tempHome, userRegistriesFile)

	for _, c := range []struct {
		sys             *types.SystemContext
		userfilePresent bool
		expected        string
	}{
		// The common case
		{nil, false, systemRegistriesConfPath},
		// There is a context, but it does not override the path.
		{&types.SystemContext{}, false, systemRegistriesConfPath},
		// Path overridden
		{&types.SystemContext{SystemRegistriesConfPath: nondefaultPath}, false, nondefaultPath},
		// Root overridden
		{
			&types.SystemContext{RootForImplicitAbsolutePaths: rootPrefix},
			false,
			filepath.Join(rootPrefix, systemRegistriesConfPath),
		},
		// Root and path overrides present simultaneously,
		{
			&types.SystemContext{
				RootForImplicitAbsolutePaths: rootPrefix,
				SystemRegistriesConfPath:     nondefaultPath,
			},
			false,
			nondefaultPath,
		},
		// User registries file overridden
		{&types.SystemContext{}, true, userRegistriesFilePath},
		// Context and user User registries file preset simultaneously
		{&types.SystemContext{SystemRegistriesConfPath: nondefaultPath}, true, nondefaultPath},
		// Root and user registries file overrides present simultaneously,
		{
			&types.SystemContext{
				RootForImplicitAbsolutePaths: rootPrefix,
				SystemRegistriesConfPath:     nondefaultPath,
			},
			true,
			nondefaultPath,
		},
		// No environment expansion happens in the overridden paths
		{&types.SystemContext{SystemRegistriesConfPath: variableReference}, false, variableReference},
	} {
		if c.userfilePresent {
			err := os.MkdirAll(filepath.Dir(userRegistriesFilePath), os.ModePerm)
			require.NoError(t, err)
			f, err := os.Create(userRegistriesFilePath)
			require.NoError(t, err)
			f.Close()
		} else {
			os.Remove(userRegistriesFilePath)
		}
		path := newConfigWrapperWithHomeDir(c.sys, tempHome).configPath
		assert.Equal(t, c.expected, path)
	}
}

func TestFindRegistry(t *testing.T) {
	sys := &types.SystemContext{
		SystemRegistriesConfPath:    "testdata/find-registry.conf",
		SystemRegistriesConfDirPath: "testdata/registries.conf.d",
	}

	registries, err := GetRegistries(sys)
	assert.Nil(t, err)
	assert.Equal(t, 19, len(registries))

	reg, err := FindRegistry(sys, "simple-prefix.com/foo/bar:latest")
	assert.Nil(t, err)
	assert.NotNil(t, reg)
	assert.Equal(t, "simple-prefix.com", reg.Prefix)
	assert.Equal(t, reg.Location, "registry.com:5000")

	// path match
	reg, err = FindRegistry(sys, "simple-prefix.com/")
	assert.Nil(t, err)
	assert.NotNil(t, reg)

	// hostname match
	reg, err = FindRegistry(sys, "simple-prefix.com")
	assert.Nil(t, err)
	assert.NotNil(t, reg)

	// subdomain prefix match
	reg, err = FindRegistry(sys, "not.so.simple-prefix.com/")
	assert.Nil(t, err)
	assert.NotNil(t, reg)
	assert.Equal(t, "subdomain-prefix.com", reg.Location)

	reg, err = FindRegistry(sys, "not.quite.simple-prefix.com/")
	assert.Nil(t, err)
	assert.NotNil(t, reg)
	assert.Equal(t, "subdomain-prefix-2.com", reg.Location)

	reg, err = FindRegistry(sys, "not.quite.simple-prefix.com:5000/with/path/and/beyond:tag")
	assert.Nil(t, err)
	assert.NotNil(t, reg)
	assert.Equal(t, "subdomain-prefix-2.com", reg.Location)

	// subdomain prefix match for *.not.quite.simple-prefix.com
	// location field overriden by /registries.conf.d/subdomain-override-1.conf
	reg, err = FindRegistry(sys, "really.not.quite.simple-prefix.com:5000/with/path/and/beyond:tag")
	assert.Nil(t, err)
	assert.NotNil(t, reg)
	assert.Equal(t, "subdomain-prefix-1-overridden-by-dropin-location.com", reg.Location)

	// In this case, the override does NOT occur because the dropin
	// prefix = "*.docker.com" which is not a match.
	reg, err = FindRegistry(sys, "foo.docker.io:5000/omg/wtf/bbq:foo")
	assert.Nil(t, err)
	assert.NotNil(t, reg)
	assert.Equal(t, "subdomain-prefix-2.com", reg.Location)

	// subdomain prefix match for *.bar.example.com
	// location field overriden by /registries.conf.d/subdomain-override-3.conf
	reg, err = FindRegistry(sys, "foo.bar.example.com:6000/omg/wtf/bbq@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	assert.Nil(t, err)
	assert.NotNil(t, reg)
	assert.Equal(t, "subdomain-prefix-3-overridden-by-dropin-location.com", reg.Location)

	// This case first matches with prefix = *.docker.io in find-registry.conf but
	// there's a longer match with *.bar.docker.io which gets used
	reg, err = FindRegistry(sys, "foo.bar.docker.io:5000/omg/wtf/bbq:foo")
	assert.Nil(t, err)
	assert.NotNil(t, reg)
	assert.Equal(t, "subdomain-prefix-4.com", reg.Location)

	// This case first matches with prefix = *.example.com in find-registry.conf but
	// there's a longer match with foo.bar.example.com:5000 which gets used
	reg, err = FindRegistry(sys, "foo.bar.example.com:5000/omg/wtf/bbq:foo")
	assert.Nil(t, err)
	assert.NotNil(t, reg)
	assert.Equal(t, "subdomain-prefix-5.com", reg.Location)

	// invalid match
	reg, err = FindRegistry(sys, "simple-prefix.comx")
	assert.Nil(t, err)
	assert.Nil(t, reg)

	reg, err = FindRegistry(sys, "complex-prefix.com:4000/with/path/and/beyond:tag")
	assert.Nil(t, err)
	assert.NotNil(t, reg)
	assert.Equal(t, "complex-prefix.com:4000/with/path", reg.Prefix)
	assert.Equal(t, "another-registry.com:5000", reg.Location)

	reg, err = FindRegistry(sys, "no-prefix.com/foo:tag")
	assert.Nil(t, err)
	assert.NotNil(t, reg)
	assert.Equal(t, "no-prefix.com", reg.Prefix)
	assert.Equal(t, "no-prefix.com", reg.Location)

	reg, err = FindRegistry(sys, "empty-prefix.com/foo:tag")
	assert.Nil(t, err)
	assert.NotNil(t, reg)
	assert.Equal(t, "empty-prefix.com", reg.Prefix)
	assert.Equal(t, "empty-prefix.com", reg.Location)

	_, err = FindRegistry(&types.SystemContext{SystemRegistriesConfPath: "testdata/this-does-not-exist.conf"}, "example.com")
	assert.Error(t, err)
}

func assertRegistryLocationsEqual(t *testing.T, expected []string, regs []Registry) {
	// verify the expected registries and their order
	names := []string{}
	for _, r := range regs {
		names = append(names, r.Location)
	}
	assert.Equal(t, expected, names)
}

func TestFindUnqualifiedSearchRegistries(t *testing.T) {
	sys := &types.SystemContext{
		SystemRegistriesConfPath:    "testdata/unqualified-search.conf",
		SystemRegistriesConfDirPath: "testdata/this-does-not-exist",
	}

	registries, err := GetRegistries(sys)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(registries))

	unqRegs, origin, err := UnqualifiedSearchRegistriesWithOrigin(sys)
	assert.Nil(t, err)
	assert.Equal(t, []string{"registry-a.com", "registry-c.com", "registry-d.com"}, unqRegs)
	assert.Equal(t, "testdata/unqualified-search.conf", origin)

	_, err = UnqualifiedSearchRegistries(&types.SystemContext{
		SystemRegistriesConfPath:    "testdata/invalid-search.conf",
		SystemRegistriesConfDirPath: "testdata/this-does-not-exist",
	})
	assert.Error(t, err)
}

func TestInvalidV2Configs(t *testing.T) {
	for _, c := range []struct{ path, errorSubstring string }{
		{"testdata/insecure-conflicts.conf", "registry 'registry.com' is defined multiple times with conflicting 'insecure' setting"},
		{"testdata/blocked-conflicts.conf", "registry 'registry.com' is defined multiple times with conflicting 'blocked' setting"},
		{"testdata/missing-mirror-location.conf", "invalid condition: mirror location is unset"},
		{"testdata/invalid-prefix.conf", "invalid location"},
		{"testdata/this-does-not-exist.conf", "no such file or directory"},
	} {
		_, err := GetRegistries(&types.SystemContext{SystemRegistriesConfPath: c.path})
		assert.Error(t, err, c.path)
		if c.errorSubstring != "" {
			assert.Contains(t, err.Error(), c.errorSubstring, c.path)
		}
	}
}

func TestUnmarshalConfig(t *testing.T) {
	registries, err := GetRegistries(&types.SystemContext{
		SystemRegistriesConfPath:    "testdata/unmarshal.conf",
		SystemRegistriesConfDirPath: "testdata/this-does-not-exist",
	})
	assert.Nil(t, err)
	assert.Equal(t, 4, len(registries))
}

func TestV1BackwardsCompatibility(t *testing.T) {
	sys := &types.SystemContext{
		SystemRegistriesConfPath:    "testdata/v1-compatibility.conf",
		SystemRegistriesConfDirPath: "testdata/this-does-not-exist",
	}

	registries, err := GetRegistries(sys)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(registries))

	unqRegs, err := UnqualifiedSearchRegistries(sys)
	assert.Nil(t, err)
	assert.Equal(t, []string{"registry-a.com", "registry-c.com", "registry-d.com"}, unqRegs)

	// check if merging works
	reg, err := FindRegistry(sys, "registry-b.com/bar/foo/barfoo:latest")
	assert.Nil(t, err)
	assert.NotNil(t, reg)
	assert.True(t, reg.Insecure)
	assert.True(t, reg.Blocked)

	for _, c := range []string{"testdata/v1-invalid-block.conf", "testdata/v1-invalid-insecure.conf", "testdata/v1-invalid-search.conf"} {
		_, err := GetRegistries(&types.SystemContext{
			SystemRegistriesConfPath:    c,
			SystemRegistriesConfDirPath: "testdata/this-does-not-exist",
		})
		assert.Error(t, err, c)
	}
}

func TestMixingV1andV2(t *testing.T) {
	_, err := GetRegistries(&types.SystemContext{
		SystemRegistriesConfPath:    "testdata/mixing-v1-v2.conf",
		SystemRegistriesConfDirPath: "testdata/this-does-not-exist",
	})
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "mixing sysregistry v1/v2 is not supported")
}

func TestConfigCache(t *testing.T) {
	configFile, err := ioutil.TempFile("", "sysregistriesv2-test")
	require.NoError(t, err)
	defer os.Remove(configFile.Name())
	defer configFile.Close()

	err = ioutil.WriteFile(configFile.Name(), []byte(`
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
insecure = true`), 0600)
	require.NoError(t, err)

	ctx := &types.SystemContext{SystemRegistriesConfPath: configFile.Name()}

	InvalidateCache()
	registries, err := GetRegistries(ctx)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(registries))

	// empty the config, but use the same SystemContext to show that the
	// previously specified registries are in the cache
	err = ioutil.WriteFile(configFile.Name(), []byte{}, 0600)
	require.NoError(t, err)
	registries, err = GetRegistries(ctx)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(registries))
}

func TestInvalidateCache(t *testing.T) {
	ctx := &types.SystemContext{SystemRegistriesConfPath: "testdata/invalidate-cache.conf"}

	InvalidateCache()
	registries, err := GetRegistries(ctx)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(registries))
	assertRegistryLocationsEqual(t, []string{"blocked.registry.com", "insecure.registry.com", "registry.com", "untrusted.registry.com"}, registries)

	// invalidate the cache, make sure it's empty and reload
	InvalidateCache()
	assert.Equal(t, 0, len(configCache))

	registries, err = GetRegistries(ctx)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(registries))
	assertRegistryLocationsEqual(t, []string{"blocked.registry.com", "insecure.registry.com", "registry.com", "untrusted.registry.com"}, registries)
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
		// Wildcard prefix examples
		{"docker.io/namespace/image", "*.io", "example.com", "example.com/namespace/image"},
		{"docker.io/library/prefix/image", "*.io", "example.com", "example.com/library/prefix/image"},
		{"sub.example.io/library/prefix/image", "*.example.io", "example.com", "example.com/library/prefix/image"},
		{"another.sub.example.io:5000/library/prefix/image:latest", "*.sub.example.io", "example.com", "example.com:5000/library/prefix/image:latest"},
		{"foo.bar.io/ns1/ns2/ns3/ns4", "*.bar.io", "omg.bbq.com/roflmao", "omg.bbq.com/roflmao/ns1/ns2/ns3/ns4"},
		// Empty location with wildcard prefix examples. Essentially, no
		// rewrite occurs and original reference is used as-is.
		{"abc.internal.registry.com/foo:bar", "*.internal.registry.com", "", "abc.internal.registry.com/foo:bar"},
		{"blah.foo.bar.com/omg:bbq", "*.com", "", "blah.foo.bar.com/omg:bbq"},
		{"alien.vs.predator.foobar.io:5000/omg", "*.foobar.io", "", "alien.vs.predator.foobar.io:5000/omg"},
		{"alien.vs.predator.foobar.io:5000/foo@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "*.foobar.io", "",
			"alien.vs.predator.foobar.io:5000/foo@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{"alien.vs.predator.foobar.io:5000/omg:bbq", "*.foobar.io", "", "alien.vs.predator.foobar.io:5000/omg:bbq"},
	} {
		ref := toNamedRef(t, c.inputRef)
		testEndpoint := Endpoint{Location: c.location}
		out, err := testEndpoint.rewriteReference(ref, c.prefix)
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
		{"docker.io/library/image", "*.com", "example.com"},
		{"foo.docker.io/library/image", "*.example.com", "example.com/image"},
		{"foo.docker.io/library/image", "*.docker.com", "example.com/image"},
	} {
		ref := toNamedRef(t, c.inputRef)
		testEndpoint := Endpoint{Location: c.location}
		out, err := testEndpoint.rewriteReference(ref, c.prefix)
		assert.NotNil(t, err)
		assert.Nil(t, out)
	}
}

func TestPullSourcesFromReference(t *testing.T) {
	sys := &types.SystemContext{
		SystemRegistriesConfPath:    "testdata/pull-sources-from-reference.conf",
		SystemRegistriesConfDirPath: "testdata/this-does-not-exist",
	}
	registries, err := GetRegistries(sys)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(registries))

	// Registry A allowing any kind of pull from mirrors
	registryA, err := FindRegistry(sys, "registry-a.com/foo/image:latest")
	assert.Nil(t, err)
	assert.NotNil(t, registryA)
	// Digest
	referenceADigest := toNamedRef(t, "registry-a.com/foo/image@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	pullSources, err := registryA.PullSourcesFromReference(referenceADigest)
	assert.Nil(t, err)
	assert.Equal(t, 3, len(pullSources))
	assert.Equal(t, "mirror-1.registry-a.com/image@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", pullSources[0].Reference.String())
	assert.True(t, pullSources[1].Endpoint.Insecure)
	// Tag
	referenceATag := toNamedRef(t, "registry-a.com/foo/image:aaa")
	pullSources, err = registryA.PullSourcesFromReference(referenceATag)
	assert.Nil(t, err)
	assert.Equal(t, 3, len(pullSources))
	assert.Equal(t, "registry-a.com/bar/image:aaa", pullSources[2].Reference.String())

	// Registry B allowing digests pull only from mirrors
	registryB, err := FindRegistry(sys, "registry-b.com/foo/image:latest")
	assert.Nil(t, err)
	assert.NotNil(t, registryB)
	// Digest
	referenceBDigest := toNamedRef(t, "registry-b.com/foo/image@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	pullSources, err = registryB.PullSourcesFromReference(referenceBDigest)
	assert.Nil(t, err)
	assert.Equal(t, 3, len(pullSources))
	assert.Equal(t, "registry-b.com/bar", pullSources[2].Endpoint.Location)
	assert.Equal(t, "registry-b.com/bar/image@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", pullSources[2].Reference.String())
	// Tag
	referenceBTag := toNamedRef(t, "registry-b.com/foo/image:aaa")
	pullSources, err = registryB.PullSourcesFromReference(referenceBTag)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(pullSources))
}

func TestTryUpdatingCache(t *testing.T) {
	ctx := &types.SystemContext{
		SystemRegistriesConfPath:    "testdata/try-update-cache-valid.conf",
		SystemRegistriesConfDirPath: "testdata/this-does-not-exist",
	}
	InvalidateCache()
	registries, err := TryUpdatingCache(ctx)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(registries.Registries))
	assert.Equal(t, 1, len(configCache))

	ctxInvalid := &types.SystemContext{
		SystemRegistriesConfPath:    "testdata/try-update-cache-invalid.conf",
		SystemRegistriesConfDirPath: "testdata/this-does-not-exist",
	}
	registries, err = TryUpdatingCache(ctxInvalid)
	assert.NotNil(t, err)
	assert.Nil(t, registries)
	assert.Equal(t, 1, len(configCache))
}

func TestRegistriesConfDirectory(t *testing.T) {
	ctx := &types.SystemContext{
		SystemRegistriesConfPath:    "testdata/base-for-registries.d.conf",
		SystemRegistriesConfDirPath: "testdata/registries.conf.d",
	}

	InvalidateCache()
	registries, err := TryUpdatingCache(ctx)
	require.NoError(t, err)
	assert.NotNil(t, registries)

	assert.Equal(t, []string{"example-overwrite.com"}, registries.UnqualifiedSearchRegistries)
	assert.Equal(t, 6, len(registries.Registries))
	assertRegistryLocationsEqual(t, []string{"subdomain-prefix-3-overridden-by-dropin-location.com", "subdomain-prefix-2-overridden-by-dropin-location.com", "subdomain-prefix-1-overridden-by-dropin-location.com", "1.com", "2.com", "base.com"}, registries.Registries)

	reg, err := FindRegistry(ctx, "base.com/test:latest")
	require.NoError(t, err)
	assert.True(t, reg.Blocked)

	usrs, origin, err := UnqualifiedSearchRegistriesWithOrigin(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"example-overwrite.com"}, usrs)
	assert.Equal(t, "testdata/registries.conf.d/config-1.conf", origin)

	// Test that unqualified-search-registries is merged correctly
	usr, err := UnqualifiedSearchRegistries(&types.SystemContext{
		SystemRegistriesConfPath:    "testdata/unqualified-search.conf",
		SystemRegistriesConfDirPath: "testdata/registries.conf.d-usr1",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"registry-a.com", "registry-c.com", "registry-d.com"}, usr) // Nothing overrides the base file

	usr, err = UnqualifiedSearchRegistries(&types.SystemContext{
		SystemRegistriesConfPath:    "testdata/unqualified-search.conf",
		SystemRegistriesConfDirPath: "testdata/registries.conf.d-usr2",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{}, usr) // Search overridden with an empty array
}

func TestParseShortNameMode(t *testing.T) {
	tests := []struct {
		input    string
		result   types.ShortNameMode
		mustFail bool
	}{
		{"disabled", types.ShortNameModeDisabled, false},
		{"enforcing", types.ShortNameModeEnforcing, false},
		{"permissive", types.ShortNameModePermissive, false},
		{"", -1, true},
		{"xxx", -1, true},
	}

	for _, test := range tests {
		shortName, err := parseShortNameMode(test.input)
		if test.mustFail {
			assert.Error(t, err)
			continue
		}
		require.NoError(t, err)
		assert.Equal(t, test.result, shortName)
	}
}

func TestGetShortNameMode(t *testing.T) {
	tests := []struct {
		path     string
		mode     types.ShortNameMode
		mustFail bool
	}{
		{
			"testdata/aliases.conf",
			types.ShortNameModeEnforcing,
			false,
		},
		{
			"testdata/registries.conf.d/config-2.conf",
			types.ShortNameModePermissive,
			false,
		},
		{
			"testdata/registries.conf.d/config-3.conf",
			types.ShortNameModePermissive, // empty -> default to permissive
			false,
		},
		{
			"testdata/invalid-short-name-mode.conf",
			-1,
			true,
		},
	}

	for _, test := range tests {
		sys := &types.SystemContext{
			SystemRegistriesConfPath:    test.path,
			SystemRegistriesConfDirPath: "testdata/this-does-not-exist",
		}
		mode, err := GetShortNameMode(sys)
		if test.mustFail {
			assert.Error(t, err)
			continue
		}
		require.NoError(t, err)
		assert.Equal(t, test.mode, mode, "%s", test.path)
	}
}

func TestCredentialHelpers(t *testing.T) {
	tests := []struct {
		confPath    string
		confDirPath string
		helpers     []string
	}{
		{
			confPath:    "testdata/cred-helper.conf",
			confDirPath: "testdata/this-does-not-exist",
			helpers:     []string{"helper-1", "helper-2"},
		},
		{
			confPath:    "testdata/empty.conf",
			confDirPath: "testdata/this-does-not-exist",
			helpers:     []string{"containers-auth.json"},
		},
		{
			confPath:    "testdata/cred-helper.conf",
			confDirPath: "testdata/registries.conf.d-empty-helpers",
			helpers:     []string{"containers-auth.json"},
		},
		{
			confPath:    "testdata/cred-helper.conf",
			confDirPath: "testdata/registries.conf.d",
			helpers:     []string{"dropin-1", "dropin-2"},
		},
	}

	for _, test := range tests {
		ctx := &types.SystemContext{
			SystemRegistriesConfPath:    test.confPath,
			SystemRegistriesConfDirPath: test.confDirPath,
		}

		helpers, err := CredentialHelpers(ctx)
		require.NoError(t, err)
		require.Equal(t, test.helpers, helpers, "%v", test)
	}
}

package sysregistriesv2

import (
	"os"
	"testing"

	"github.com/containers/image/v5/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShortNameAliasConfNonempty(t *testing.T) {
	for _, c := range []shortNameAliasConf{
		{},
		{Aliases: map[string]string{}},
	} {
		copy := c // A shallow copy
		res := copy.nonempty()
		assert.False(t, res, c)
		assert.Equal(t, c, copy, c) // Ensure the method did not change the original value
	}

	res := (&shortNameAliasConf{}).nonempty()
	assert.False(t, res)
	for _, c := range []shortNameAliasConf{
		{Aliases: map[string]string{"a": "example.com/b"}},
	} {
		copy := c // A shallow copy
		res := copy.nonempty()
		assert.True(t, res, c)
		assert.Equal(t, c, copy, c) // Ensure the method did not change the original value
	}
}

func TestParseShortNameValue(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		// VALID INPUT
		{"docker.io/library/fedora", true},
		{"localhost/fedora", true},
		{"localhost:5000/fedora", true},
		{"localhost:5000/namespace/fedora", true},
		// INVALID INPUT
		{"docker.io/library/fedora:latest", false}, // tag
		{"docker.io/library/fedora@sha256:b87dd5f837112a9e1e9882963a6406387597698268c0ad371b187151a5dfe6bf", false}, // digest
		{"fedora", false},                // short name
		{"fedora:latest", false},         // short name + tag
		{"library/fedora", false},        // no registry
		{"library/fedora:latest", false}, // no registry + tag
		{"$$4455%%", false},              // garbage
		{"docker://foo", false},          // transports are not supported
		{"docker-archive://foo", false},  // transports are not supported
		{"", false},                      // empty
	}

	for _, test := range tests {
		named, err := parseShortNameValue(test.input)
		if test.valid {
			require.NoError(t, err, "%q should be a valid alias", test.input)
			assert.NotNil(t, named)
			assert.Equal(t, test.input, named.String())
		} else {
			require.Error(t, err, "%q should be an invalid alias", test.input)
			assert.Nil(t, named)
		}
	}

	// Now make sure that docker.io references are normalized.
	named, err := parseShortNameValue("docker.io/fedora")
	require.NoError(t, err)
	assert.NotNil(t, named)
	assert.Equal(t, "docker.io/library/fedora", named.String())
}

func TestValidateShortName(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		// VALID INPUT
		{"library/fedora", true},
		{"fedora", true},
		{"1234567489", true},
		// INVALID INPUT
		{"docker.io/library/fedora:latest", false},
		{"docker.io/library/fedora@sha256:b87dd5f837112a9e1e9882963a6406387597698268c0ad371b187151a5dfe6bf", false}, // digest
		{"fedora:latest", false},
		{"library/fedora:latest", false},
		{"$$4455%%", false},
		{"docker://foo", false},
		{"docker-archive://foo", false},
		{"", false},
	}

	for _, test := range tests {
		err := validateShortName(test.input)
		if test.valid {
			require.NoError(t, err, "%q should be a valid alias", test.input)
		} else {
			require.Error(t, err, "%q should be an invalid alias", test.input)
		}
	}
}

func TestResolveShortNameAlias(t *testing.T) {
	tmp, err := os.CreateTemp("", "aliases.conf")
	require.NoError(t, err)
	defer os.Remove(tmp.Name())

	sys := &types.SystemContext{
		SystemRegistriesConfPath:    "testdata/aliases.conf",
		SystemRegistriesConfDirPath: "testdata/this-does-not-exist",
		UserShortNameAliasConfPath:  tmp.Name(),
	}

	InvalidateCache()
	conf, err := tryUpdatingCache(sys, newConfigWrapper(sys))
	require.NoError(t, err)
	assert.Len(t, conf.aliasCache.namedAliases, 4)
	assert.Len(t, conf.partialV2.Aliases, 0) // This is an implementation detail, not an API guarantee.

	aliases := []struct {
		name, value string
	}{
		{
			"docker",
			"docker.io/library/foo",
		},
		{
			"quay/foo",
			"quay.io/library/foo",
		},
		{
			"example",
			"example.com/library/foo",
		},
	}

	for _, alias := range aliases {
		value, path, err := ResolveShortNameAlias(sys, alias.name)
		require.NoError(t, err)
		require.NotNil(t, value)
		assert.Equal(t, alias.value, value.String())
		assert.Equal(t, "testdata/aliases.conf", path)
	}

	// Non-existent alias.
	value, path, err := ResolveShortNameAlias(sys, "idonotexist")
	require.NoError(t, err)
	assert.Nil(t, value)
	assert.Equal(t, "", path)

	// Empty right-hand value (special case) -> does not resolve.
	value, path, err = ResolveShortNameAlias(sys, "empty")
	require.NoError(t, err)
	assert.Nil(t, value)
	assert.Equal(t, "testdata/aliases.conf", path)
}

func TestAliasesWithDropInConfigs(t *testing.T) {
	tmp, err := os.CreateTemp("", "aliases.conf")
	require.NoError(t, err)
	defer os.Remove(tmp.Name())

	sys := &types.SystemContext{
		SystemRegistriesConfPath:    "testdata/aliases.conf",
		SystemRegistriesConfDirPath: "testdata/registries.conf.d",
		UserShortNameAliasConfPath:  tmp.Name(),
	}

	InvalidateCache()
	conf, err := tryUpdatingCache(sys, newConfigWrapper(sys))
	require.NoError(t, err)
	assert.Len(t, conf.aliasCache.namedAliases, 8)
	assert.Len(t, conf.partialV2.Aliases, 0) // This is an implementation detail, not an API guarantee.

	aliases := []struct {
		name, value, config string
	}{
		{
			"docker",
			"docker.io/library/config1",
			"testdata/registries.conf.d/config-1.conf",
		},
		{
			"quay/foo",
			"quay.io/library/foo",
			"testdata/aliases.conf",
		},
		{
			"config1",
			"config1.com/image", // from config1
			"testdata/registries.conf.d/config-1.conf",
		},
		{
			"barz",
			"barz.com/config2", // from config1, overridden by config2
			"testdata/registries.conf.d/config-2.conf",
		},
		{
			"config2",
			"config2.com/image", // from config2
			"testdata/registries.conf.d/config-2.conf",
		},
		{
			"added1",
			"aliases.conf/added1", // from AddShortNameAlias
			tmp.Name(),
		},
		{
			"added2",
			"aliases.conf/added2", // from AddShortNameAlias
			tmp.Name(),
		},
		{
			"added3",
			"aliases.conf/added3", // from config2, overridden by AddShortNameAlias
			tmp.Name(),
		},
	}

	require.NoError(t, AddShortNameAlias(sys, "added1", "aliases.conf/added1"))
	require.NoError(t, AddShortNameAlias(sys, "added2", "aliases.conf/added2"))
	require.NoError(t, AddShortNameAlias(sys, "added3", "aliases.conf/added3"))

	for _, alias := range aliases {
		value, path, err := ResolveShortNameAlias(sys, alias.name)
		require.NoError(t, err)
		require.NotNil(t, value, "%v", alias)
		assert.Equal(t, alias.value, value.String())
		assert.Equal(t, alias.config, path)
	}

	value, path, err := ResolveShortNameAlias(sys, "i/do/no/exist")
	require.NoError(t, err)
	assert.Nil(t, value)
	assert.Equal(t, "", path)

	// Empty right-hand value (special case) -> does not resolve.
	value, path, err = ResolveShortNameAlias(sys, "empty") // from aliases.conf, overridden by config2
	require.NoError(t, err)
	assert.Nil(t, value)
	assert.Equal(t, "testdata/aliases.conf", path)

	mode, err := GetShortNameMode(sys)
	require.NoError(t, err)
	assert.Equal(t, types.ShortNameModePermissive, mode) // from alias.conf, overridden by config2

	// Now remove the aliases from the machine config.
	require.NoError(t, RemoveShortNameAlias(sys, "added1"))
	require.NoError(t, RemoveShortNameAlias(sys, "added2"))
	require.NoError(t, RemoveShortNameAlias(sys, "added3"))

	// Make sure that 1 and 2 are gone.
	for _, alias := range []string{"added1", "added2"} {
		value, path, err := ResolveShortNameAlias(sys, alias)
		require.NoError(t, err)
		assert.Nil(t, value)
		assert.Equal(t, "", path)
	}

	// 3 is still present in config2
	value, path, err = ResolveShortNameAlias(sys, "added3")
	require.NoError(t, err)
	require.NotNil(t, value)
	assert.Equal(t, "xxx.com/image", value.String())
	assert.Equal(t, "testdata/registries.conf.d/config-2.conf", path)

	require.Error(t, RemoveShortNameAlias(sys, "added3")) // we cannot remove it from config2
}

func TestInvalidAliases(t *testing.T) {
	tmp, err := os.CreateTemp("", "aliases.conf")
	require.NoError(t, err)
	defer os.Remove(tmp.Name())

	sys := &types.SystemContext{
		SystemRegistriesConfPath:    "testdata/invalid-aliases.conf",
		SystemRegistriesConfDirPath: "testdata/this-does-not-exist",
		UserShortNameAliasConfPath:  tmp.Name(),
	}

	InvalidateCache()
	_, err = TryUpdatingCache(sys)
	require.Error(t, err)

	// We validate the alias value before loading existing configuration,
	// so this tests the validation although the pre-existing configuration
	// is invalid.
	assert.Error(t, AddShortNameAlias(sys, "added1", "aliases"))
	assert.Error(t, AddShortNameAlias(sys, "added2", "aliases.conf"))
	assert.Error(t, AddShortNameAlias(sys, "added3", ""))
	assert.Error(t, AddShortNameAlias(sys, "added3", " "))
	assert.Error(t, AddShortNameAlias(sys, "added3", "$$$"))
}

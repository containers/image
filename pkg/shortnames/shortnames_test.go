package shortnames

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/pkg/sysregistriesv2"
	"github.com/containers/image/v5/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsShortName(t *testing.T) {
	tests := []struct {
		input                      string
		parseUnnormalizedShortName bool
		mustFail                   bool
	}{
		// SHORT NAMES
		{"fedora", true, false},
		{"fedora:latest", true, false},
		{"library/fedora", true, false},
		{"library/fedora:latest", true, false},
		{"busybox@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a", true, false},
		{"busybox:latest@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a", true, false},
		// !SHORT NAMES
		{"quay.io/fedora", false, false},
		{"docker.io/fedora", false, false},
		{"docker.io/library/fedora:latest", false, false},
		{"localhost/fedora", false, false},
		{"localhost:5000/fedora:latest", false, false},
		{"example.foo.this.may.be.garbage.but.maybe.not:1234/fedora:latest", false, false},
		{"docker.io/library/busybox@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a", false, false},
		{"docker.io/library/busybox:latest@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a", false, false},
		{"docker.io/fedora@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a", false, false},
		// INVALID NAMES
		{"", false, true},
		{"$$$", false, true},
		{"::", false, true},
		{"docker://quay.io/library/foo:bar", false, true},
		{" ", false, true},
	}

	for _, test := range tests {
		res, _, err := parseUnnormalizedShortName(test.input)
		if test.mustFail {
			require.Error(t, err, "%q should not be parseable")
			continue
		}
		require.NoError(t, err, "%q should be parseable")
		assert.Equal(t, test.parseUnnormalizedShortName, res, "%q", test.input)
	}
}

func TestSplitUserInput(t *testing.T) {
	tests := []struct {
		input      string
		repo       string
		isTagged   bool
		isDigested bool
	}{
		// Neither tags nor digests
		{"fedora", "fedora", false, false},
		{"repo/fedora", "repo/fedora", false, false},
		{"registry.com/fedora", "registry.com/fedora", false, false},
		// Tags
		{"fedora:tag", "fedora", true, false},
		{"repo/fedora:tag", "repo/fedora", true, false},
		{"registry.com/fedora:latest", "registry.com/fedora", true, false},
		// Digests
		{"fedora@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a", "fedora", false, true},
		{"repo/fedora@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a", "repo/fedora", false, true},
		{"registry.com/fedora@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a", "registry.com/fedora", false, true},
	}

	for _, test := range tests {
		_, ref, err := parseUnnormalizedShortName(test.input)
		require.NoError(t, err, "%v", test)

		isTagged, isDigested, shortNameRepo, tag, digest := splitUserInput(ref)
		require.NotNil(t, shortNameRepo)
		normalized := shortNameRepo.String()
		assert.Equal(t, test.repo, normalized)
		assert.Equal(t, test.isTagged, isTagged)
		assert.Equal(t, test.isDigested, isDigested)
		if isTagged {
			normalized = normalized + ":" + tag
		} else if isDigested {
			normalized = normalized + "@" + digest.String()
		}
		assert.Equal(t, test.input, normalized)
	}
}

func TestResolve(t *testing.T) {
	tmp, err := ioutil.TempFile("", "aliases.conf")
	require.NoError(t, err)
	defer os.Remove(tmp.Name())

	sys := &types.SystemContext{
		SystemRegistriesConfPath:    "testdata/aliases.conf",
		SystemRegistriesConfDirPath: "testdata/this-does-not-exist",
		UserShortNameAliasConfPath:  tmp.Name(),
	}

	_, err = sysregistriesv2.TryUpdatingCache(sys)
	require.NoError(t, err)

	tests := []struct {
		name, value string
	}{
		{"docker", "docker.io/library/foo:latest"},
		{"docker:tag", "docker.io/library/foo:tag"},
		{
			"docker@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a",
			"docker.io/library/foo@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a",
		},
		{"quay/foo", "quay.io/library/foo:latest"},
		{"quay/foo:tag", "quay.io/library/foo:tag"},
		{
			"quay/foo@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a",
			"quay.io/library/foo@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a",
		},
		{"example", "example.com/library/foo:latest"},
		{"example:tag", "example.com/library/foo:tag"},
		{
			"example@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a",
			"example.com/library/foo@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a",
		},
	}

	// All of them should resolve correctly.
	for _, test := range tests {
		resolved, err := Resolve(sys, test.name)
		require.NoError(t, err, "%v", test)
		require.NotNil(t, resolved)
		require.Len(t, resolved.PullCandidates, 1)
		assert.Equal(t, test.value, resolved.PullCandidates[0].Value.String())
		assert.False(t, resolved.PullCandidates[0].record)
	}

	// Non-existent should return an error as no search registries are
	// configured in the config.
	resolved, err := Resolve(sys, "doesnotexist")
	require.Error(t, err)
	require.Nil(t, resolved)

	// An empty name is not valid.
	resolved, err = Resolve(sys, "")
	require.Error(t, err)
	require.Nil(t, resolved)

	// Invalid input.
	resolved, err = Resolve(sys, "Invalid#$")
	require.Error(t, err)
	require.Nil(t, resolved)

	// Fully-qualified input will be returned as is.
	resolved, err = Resolve(sys, "quay.io/repo/fedora")
	require.NoError(t, err)
	require.NotNil(t, resolved)
	require.Len(t, resolved.PullCandidates, 1)
	assert.Equal(t, "quay.io/repo/fedora:latest", resolved.PullCandidates[0].Value.String())
	assert.False(t, resolved.PullCandidates[0].record)
}

func toNamed(t *testing.T, input string, trim bool) reference.Named {
	ref, err := reference.Parse(input)
	require.NoError(t, err)
	named := ref.(reference.Named)
	require.NotNil(t, named)

	if trim {
		named = reference.TrimNamed(named)
	}

	return named
}

func addAlias(t *testing.T, sys *types.SystemContext, name string, value string, mustFail bool) {
	namedValue := toNamed(t, value, false)

	if mustFail {
		require.Error(t, Add(sys, name, namedValue))
	} else {
		require.NoError(t, Add(sys, name, namedValue))
	}
}

func removeAlias(t *testing.T, sys *types.SystemContext, name string, mustFail bool, trim bool) {
	namedName := toNamed(t, name, trim)

	if mustFail {
		require.Error(t, Remove(sys, namedName.String()))
	} else {
		require.NoError(t, Remove(sys, namedName.String()))
	}
}

func TestResolveWithDropInConfigs(t *testing.T) {
	tmp, err := ioutil.TempFile("", "aliases.conf")
	require.NoError(t, err)
	defer os.Remove(tmp.Name())

	sys := &types.SystemContext{
		SystemRegistriesConfPath:    "testdata/aliases.conf",
		SystemRegistriesConfDirPath: "testdata/registries.conf.d",
		UserShortNameAliasConfPath:  tmp.Name(),
	}

	_, err = sysregistriesv2.TryUpdatingCache(sys)
	require.NoError(t, err)

	tests := []struct {
		name, value string
	}{
		{"docker", "docker.io/library/config1:latest"}, // overridden by config1
		{"docker:tag", "docker.io/library/config1:tag"},
		{
			"docker@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a",
			"docker.io/library/config1@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a",
		},
		{"quay/foo", "quay.io/library/foo:latest"},
		{"quay/foo:tag", "quay.io/library/foo:tag"},
		{
			"quay/foo@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a",
			"quay.io/library/foo@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a",
		},
		{"config1", "config1.com/image:latest"},
		{"config1:tag", "config1.com/image:tag"},
		{
			"config1@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a",
			"config1.com/image@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a",
		},
		{"barz", "barz.com/config2:latest"}, // from config1, overridden by config2
		{"barz:tag", "barz.com/config2:tag"},
		{
			"barz@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a",
			"barz.com/config2@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a",
		},
		{"added1", "aliases.conf/added1:latest"}, // from Add()
		{"added1:tag", "aliases.conf/added1:tag"},
		{
			"added1@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a",
			"aliases.conf/added1@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a",
		},
		{"added2", "aliases.conf/added2:latest"}, // from Add()
		{"added2:tag", "aliases.conf/added2:tag"},
		{
			"added2@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a",
			"aliases.conf/added2@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a",
		},
		{"added3", "aliases.conf/added3:latest"}, // from Add()
		{"added3:tag", "aliases.conf/added3:tag"},
		{
			"added3@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a",
			"aliases.conf/added3@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a",
		},
	}

	addAlias(t, sys, "added1", "aliases.conf/added1", false)
	addAlias(t, sys, "added2", "aliases.conf/added2", false)
	addAlias(t, sys, "added3", "aliases.conf/added3", false)

	// Tags/digests are invalid!
	addAlias(t, sys, "added3", "aliases.conf/added3:tag", true)
	addAlias(t, sys, "added3:tag", "aliases.conf/added3", true)
	addAlias(t, sys, "added3", "aliases.conf/added3@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a", true)
	addAlias(t, sys, "added3@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a", "aliases.conf/added3", true)

	// All of them should resolve correctly.
	for _, test := range tests {
		resolved, err := Resolve(sys, test.name)
		require.NoError(t, err)
		require.NotNil(t, resolved)
		require.Len(t, resolved.PullCandidates, 1)
		assert.Equal(t, test.value, resolved.PullCandidates[0].Value.String())
		assert.False(t, resolved.PullCandidates[0].record)
	}

	// config1 sets one search registry.
	resolved, err := Resolve(sys, "doesnotexist")
	require.NoError(t, err)
	require.NotNil(t, resolved)
	require.Len(t, resolved.PullCandidates, 1)
	assert.Equal(t, "example-overwrite.com/doesnotexist:latest", resolved.PullCandidates[0].Value.String())

	// An empty name is not valid.
	resolved, err = Resolve(sys, "")
	require.Error(t, err)
	require.Nil(t, resolved)

	// Invalid input.
	resolved, err = Resolve(sys, "Invalid#$")
	require.Error(t, err)
	require.Nil(t, resolved)

	// Fully-qualified input will be returned as is.
	resolved, err = Resolve(sys, "quay.io/repo/fedora")
	require.NoError(t, err)
	require.NotNil(t, resolved)
	require.Len(t, resolved.PullCandidates, 1)
	assert.Equal(t, "quay.io/repo/fedora:latest", resolved.PullCandidates[0].Value.String())
	assert.False(t, resolved.PullCandidates[0].record)

	resolved, err = Resolve(sys, "localhost/repo/fedora:sometag")
	require.NoError(t, err)
	require.NotNil(t, resolved)
	require.Len(t, resolved.PullCandidates, 1)
	assert.Equal(t, "localhost/repo/fedora:sometag", resolved.PullCandidates[0].Value.String())
	assert.False(t, resolved.PullCandidates[0].record)

	// Now test removal.

	// Stored in aliases.conf, so we can remove it.
	removeAlias(t, sys, "added1", false, false)
	removeAlias(t, sys, "added2", false, false)
	removeAlias(t, sys, "added3", false, false)
	removeAlias(t, sys, "added2:tag", true, false)
	removeAlias(t, sys, "added3@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a", true, false)

	// Doesn't exist -> error.
	removeAlias(t, sys, "added1", true, false)
	removeAlias(t, sys, "added2", true, false)
	removeAlias(t, sys, "added3", true, false)

	// Cannot remove entries from registries.conf files -> error.
	removeAlias(t, sys, "docker", true, false)
	removeAlias(t, sys, "docker", true, false)
	removeAlias(t, sys, "docker", true, false)
}

func TestResolveWithVaryingShortNameModes(t *testing.T) {
	tmp, err := ioutil.TempFile("", "aliases.conf")
	require.NoError(t, err)
	defer os.Remove(tmp.Name())

	tests := []struct {
		confPath   string
		mode       types.ShortNameMode
		name       string
		mustFail   bool
		numAliases int
	}{
		// Invalid -> error
		{"testdata/no-reg.conf", types.ShortNameModeInvalid, "repo/image", true, 0},
		{"testdata/one-reg.conf", types.ShortNameModeInvalid, "repo/image", true, 0},
		{"testdata/two-reg.conf", types.ShortNameModeInvalid, "repo/image", true, 0},
		// Permissive + match -> return alias
		{"testdata/no-reg.conf", types.ShortNameModePermissive, "repo/image", false, 1},
		{"testdata/one-reg.conf", types.ShortNameModePermissive, "repo/image", false, 1},
		{"testdata/two-reg.conf", types.ShortNameModePermissive, "repo/image", false, 1},
		// Permissive + no match -> search (no tty)
		{"testdata/no-reg.conf", types.ShortNameModePermissive, "doesnotexist", true, 0},
		{"testdata/one-reg.conf", types.ShortNameModePermissive, "doesnotexist", false, 1},
		{"testdata/two-reg.conf", types.ShortNameModePermissive, "doesnotexist", false, 2},
		// Disabled + match -> return alias
		{"testdata/no-reg.conf", types.ShortNameModeDisabled, "repo/image", false, 1},
		{"testdata/one-reg.conf", types.ShortNameModeDisabled, "repo/image", false, 1},
		{"testdata/two-reg.conf", types.ShortNameModeDisabled, "repo/image", false, 1},
		// Disabled + no match -> search
		{"testdata/no-reg.conf", types.ShortNameModeDisabled, "doesnotexist", true, 0},
		{"testdata/one-reg.conf", types.ShortNameModeDisabled, "doesnotexist", false, 1},
		{"testdata/two-reg.conf", types.ShortNameModeDisabled, "doesnotexist", false, 2},
		// Enforcing + match -> return alias
		{"testdata/no-reg.conf", types.ShortNameModeEnforcing, "repo/image", false, 1},
		{"testdata/one-reg.conf", types.ShortNameModeEnforcing, "repo/image", false, 1},
		{"testdata/two-reg.conf", types.ShortNameModeEnforcing, "repo/image", false, 1},
		// Enforcing + no match -> error if search regs > 1 and no tty
		{"testdata/no-reg.conf", types.ShortNameModeEnforcing, "doesnotexist", true, 0},
		{"testdata/one-reg.conf", types.ShortNameModeEnforcing, "doesnotexist", false, 1},
		{"testdata/two-reg.conf", types.ShortNameModeEnforcing, "doesnotexist", true, 0},
	}

	for _, test := range tests {
		sys := &types.SystemContext{
			SystemRegistriesConfDirPath: "testdata/this-does-not-exist",
			UserShortNameAliasConfPath:  tmp.Name(),
			// From test
			SystemRegistriesConfPath: test.confPath,
			ShortNameMode:            &test.mode,
		}

		_, err := sysregistriesv2.TryUpdatingCache(sys)
		require.NoError(t, err)

		resolved, err := Resolve(sys, test.name)
		if test.mustFail {
			require.Error(t, err, "%v", test)
			continue
		}
		require.NoError(t, err, "%v", test)
		require.NotNil(t, resolved)
		require.Len(t, resolved.PullCandidates, test.numAliases, "%v", test)
	}
}

func TestResolveAndRecord(t *testing.T) {
	tmp, err := ioutil.TempFile("", "aliases.conf")
	require.NoError(t, err)
	defer os.Remove(tmp.Name())

	sys := &types.SystemContext{
		SystemRegistriesConfPath:    "testdata/two-reg.conf",
		SystemRegistriesConfDirPath: "testdata/this-does-not-exist",
		UserShortNameAliasConfPath:  tmp.Name(),
	}

	_, err = sysregistriesv2.TryUpdatingCache(sys)
	require.NoError(t, err)

	tests := []struct {
		name     string
		expected []string
	}{
		// No alias -> USRs
		{"foo", []string{"quay.io/foo:latest", "registry.com/foo:latest"}},
		{"foo:tag", []string{"quay.io/foo:tag", "registry.com/foo:tag"}},
		{"foo@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a", []string{"quay.io/foo@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a", "registry.com/foo@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a"}},
		{"repo/foo", []string{"quay.io/repo/foo:latest", "registry.com/repo/foo:latest"}},
		{"repo/foo:tag", []string{"quay.io/repo/foo:tag", "registry.com/repo/foo:tag"}},
		{"repo/foo@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a", []string{"quay.io/repo/foo@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a", "registry.com/repo/foo@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a"}},
		// Alias
		{"repo/image", []string{"quay.io/repo/image:latest"}},
		{"repo/image:tag", []string{"quay.io/repo/image:tag"}},
		{"repo/image@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a", []string{"quay.io/repo/image@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a"}},
	}
	for _, test := range tests {
		resolved, err := Resolve(sys, test.name)
		require.NoError(t, err, "%v", test)
		require.NotNil(t, resolved)
		require.Len(t, resolved.PullCandidates, len(test.expected), "%v", test)

		for i, candidate := range resolved.PullCandidates {
			require.Equal(t, test.expected[i], candidate.Value.String(), "%v", test)

			require.False(t, candidate.record, "%v", test)
			candidate.record = true // make sure we can actually record

			// Record the alias, look it up another time and make
			// sure there's only one match (i.e., the new alias)
			// and that is has the expected value.
			require.NoError(t, candidate.Record())

			newResolved, err := Resolve(sys, test.name)
			require.NoError(t, err, "%v", test)
			require.Len(t, newResolved.PullCandidates, 1, "%v", test)
			require.Equal(t, candidate.Value.String(), newResolved.PullCandidates[0].Value.String(), "%v", test)

			// Now remove the alias again.
			removeAlias(t, sys, test.name, false, true)

			// Now set recording to false and try recording again.
			candidate.record = false
			require.NoError(t, candidate.Record())
			removeAlias(t, sys, test.name, true, true) // must error out now
		}
	}
}

func TestResolveLocally(t *testing.T) {
	tmp, err := ioutil.TempFile("", "aliases.conf")
	require.NoError(t, err)
	defer os.Remove(tmp.Name())

	sys := &types.SystemContext{
		SystemRegistriesConfPath:    "testdata/two-reg.conf",
		SystemRegistriesConfDirPath: "testdata/this-does-not-exist",
		UserShortNameAliasConfPath:  tmp.Name(),
	}

	aliases, err := ResolveLocally(sys, "repo/image") // alias match
	require.NoError(t, err)
	require.Len(t, aliases, 4)                                             // alias + localhost + two regs
	assert.Equal(t, "quay.io/repo/image:latest", aliases[0].String())      // alias
	assert.Equal(t, "localhost/repo/image:latest", aliases[1].String())    // localhost
	assert.Equal(t, "quay.io/repo/image:latest", aliases[2].String())      // registry 0
	assert.Equal(t, "registry.com/repo/image:latest", aliases[3].String()) // registry 0

	aliases, err = ResolveLocally(sys, "foo") // no alias match
	require.NoError(t, err)
	require.Len(t, aliases, 3)                                      // localhost + two regs
	assert.Equal(t, "localhost/foo:latest", aliases[0].String())    // localhost
	assert.Equal(t, "quay.io/foo:latest", aliases[1].String())      // registry 0
	assert.Equal(t, "registry.com/foo:latest", aliases[2].String()) // registry 0

	aliases, err = ResolveLocally(sys, "foo:tag") // no alias match tagged
	require.NoError(t, err)
	require.Len(t, aliases, 3)                                   // localhost + two regs
	assert.Equal(t, "localhost/foo:tag", aliases[0].String())    // localhost
	assert.Equal(t, "quay.io/foo:tag", aliases[1].String())      // registry 0
	assert.Equal(t, "registry.com/foo:tag", aliases[2].String()) // registry 0

	aliases, err = ResolveLocally(sys, "foo@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a") // no alias match digested
	require.NoError(t, err)
	require.Len(t, aliases, 3)                                                                                                       // localhost + two regs
	assert.Equal(t, "localhost/foo@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a", aliases[0].String())    // localhost
	assert.Equal(t, "quay.io/foo@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a", aliases[1].String())      // registry 0
	assert.Equal(t, "registry.com/foo@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a", aliases[2].String()) // registry 0

	aliases, err = ResolveLocally(sys, "localhost/foo") // localhost
	require.NoError(t, err)
	require.Len(t, aliases, 1)
	assert.Equal(t, "localhost/foo:latest", aliases[0].String())

	aliases, err = ResolveLocally(sys, "localhost/foo:tag") // localhost + tag
	require.NoError(t, err)
	require.Len(t, aliases, 1)
	assert.Equal(t, "localhost/foo:tag", aliases[0].String())

	aliases, err = ResolveLocally(sys, "localhost/foo@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a") // localhost + digest
	require.NoError(t, err)
	require.Len(t, aliases, 1)
	assert.Equal(t, "localhost/foo@sha256:d366a4665ab44f0648d7a00ae3fae139d55e32f9712c67accd604bb55df9d05a", aliases[0].String())
}

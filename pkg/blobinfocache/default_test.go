package blobinfocache

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/containers/image/v5/pkg/blobinfocache/memory"
	"github.com/containers/image/v5/pkg/blobinfocache/sqlite"
	"github.com/containers/image/v5/types"
	"github.com/stretchr/testify/assert"
)

func TestBlobInfoCacheDir(t *testing.T) {
	const nondefaultDir = "/this/is/not/the/default/cache/dir"
	const rootPrefix = "/root/prefix"
	const homeDir = "/fake/home/directory"
	const xdgDataHome = "/fake/home/directory/XDG"

	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_DATA_HOME", xdgDataHome)

	// The default paths and explicit overrides
	for _, c := range []struct {
		sys      *types.SystemContext
		euid     int
		expected string
	}{
		// The common case
		{nil, 0, systemBlobInfoCacheDir},
		{nil, 1, filepath.Join(xdgDataHome, "containers", "cache")},
		// There is a context, but it does not override the path.
		{&types.SystemContext{}, 0, systemBlobInfoCacheDir},
		{&types.SystemContext{}, 1, filepath.Join(xdgDataHome, "containers", "cache")},
		// Path overridden
		{&types.SystemContext{BlobInfoCacheDir: nondefaultDir}, 0, nondefaultDir},
		{&types.SystemContext{BlobInfoCacheDir: nondefaultDir}, 1, nondefaultDir},
		// Root overridden
		{&types.SystemContext{RootForImplicitAbsolutePaths: rootPrefix}, 0, filepath.Join(rootPrefix, systemBlobInfoCacheDir)},
		{&types.SystemContext{RootForImplicitAbsolutePaths: rootPrefix}, 1, filepath.Join(xdgDataHome, "containers", "cache")},
		// Root and path overrides present simultaneously,
		{
			&types.SystemContext{
				RootForImplicitAbsolutePaths: rootPrefix,
				BlobInfoCacheDir:             nondefaultDir,
			},
			0, nondefaultDir,
		},
		{
			&types.SystemContext{
				RootForImplicitAbsolutePaths: rootPrefix,
				BlobInfoCacheDir:             nondefaultDir,
			},
			1, nondefaultDir,
		},
	} {
		path, err := blobInfoCacheDir(c.sys, c.euid)
		require.NoError(t, err)
		assert.Equal(t, c.expected, path)
	}

	// Paths used by unprivileged users
	for caseIndex, c := range []struct {
		xdgDH, home, expected string
	}{
		{"", homeDir, filepath.Join(homeDir, ".local", "share", "containers", "cache")}, // HOME only
		{xdgDataHome, "", filepath.Join(xdgDataHome, "containers", "cache")},            // XDG_DATA_HOME only
		{xdgDataHome, homeDir, filepath.Join(xdgDataHome, "containers", "cache")},       // both
		{"", "", ""}, // neither
	} {
		t.Run(fmt.Sprintf("unprivileged %d", caseIndex), func(t *testing.T) {
			// Always use t.Setenv() to ensure the environment variable is restored to the original value after the test.
			// Then, in cases where the test needs the variable unset (not just set to empty), use a raw os.Unsetenv()
			// to override the situation. (Sadly there isn’t a t.Unsetenv() as of Go 1.17.)
			t.Setenv("XDG_DATA_HOME", c.xdgDH)
			if c.xdgDH == "" {
				os.Unsetenv("XDG_DATA_HOME")
			}
			t.Setenv("HOME", c.home)
			if c.home == "" {
				os.Unsetenv("HOME")
			}
			for _, sys := range []*types.SystemContext{nil, {}} {
				path, err := blobInfoCacheDir(sys, 1)
				if c.expected != "" {
					require.NoError(t, err)
					assert.Equal(t, c.expected, path)
				} else {
					assert.Error(t, err)
				}
			}
		})
	}
}

func TestDefaultCache(t *testing.T) {
	tmpDir := t.TempDir()

	// Success
	normalDir := filepath.Join(tmpDir, "normal")
	c := DefaultCache(&types.SystemContext{BlobInfoCacheDir: normalDir})
	// This is ugly hard-coding internals of sqlite.cache
	sqliteCache, err := sqlite.New(filepath.Join(normalDir, blobInfoCacheFilename))
	require.NoError(t, err)
	assert.Equal(t, sqliteCache, c)

	// Error running blobInfoCacheDir:
	// Use t.Setenv() just as a way to set up cleanup to original values; then os.Unsetenv() to test a situation where the values are not set.
	t.Setenv("HOME", "")
	os.Unsetenv("HOME")
	t.Setenv("XDG_DATA_HOME", "")
	os.Unsetenv("XDG_DATA_HOME")
	c = DefaultCache(nil)
	assert.IsType(t, memory.New(), c)

	// Error creating the parent directory:
	unwritableDir := filepath.Join(tmpDir, "unwritable")
	err = os.Mkdir(unwritableDir, 0o700)
	require.NoError(t, err)
	defer func() {
		err = os.Chmod(unwritableDir, 0o700) // To make it possible to remove it again
		require.NoError(t, err)
	}()
	err = os.Chmod(unwritableDir, 0o500)
	require.NoError(t, err)
	st, _ := os.Stat(unwritableDir)
	logrus.Errorf("%s: %#v", unwritableDir, st)
	c = DefaultCache(&types.SystemContext{BlobInfoCacheDir: filepath.Join(unwritableDir, "subdirectory")})
	assert.IsType(t, memory.New(), c)
}

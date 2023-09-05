package sqlite

import (
	"path/filepath"
	"testing"

	"github.com/containers/image/v5/internal/blobinfocache"
	"github.com/containers/image/v5/pkg/blobinfocache/internal/test"
	"github.com/stretchr/testify/require"
)

var _ blobinfocache.BlobInfoCache2 = &cache{}

func newTestCache(t *testing.T) blobinfocache.BlobInfoCache2 {
	dir := t.TempDir()
	cache, err := new2(filepath.Join(dir, "db.sqlite"))
	require.NoError(t, err)
	return cache
}

func TestNew(t *testing.T) {
	test.GenericCache(t, newTestCache)
}

// FIXME: Tests for the various corner cases / failure cases of sqlite.cache should be added here.

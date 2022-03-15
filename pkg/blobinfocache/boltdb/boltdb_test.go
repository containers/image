package boltdb

import (
	"path/filepath"
	"testing"

	"github.com/containers/image/v5/internal/blobinfocache"
	"github.com/containers/image/v5/pkg/blobinfocache/internal/test"
)

var _ blobinfocache.BlobInfoCache2 = &cache{}

func newTestCache(t *testing.T) (blobinfocache.BlobInfoCache2, func(t *testing.T)) {
	// We need a separate temporary directory here, because bolt.Open(…, &bolt.Options{Readonly:true}) can't deal with
	// an existing but empty file, and incorrectly fails without releasing the lock - which in turn causes
	// any future writes to hang.  Creating a temporary directory allows us to use a path to a
	// non-existent file, thus replicating the expected conditions for creating a new DB.
	dir := t.TempDir()
	return new2(filepath.Join(dir, "db")), func(t *testing.T) {}
}

func TestNew(t *testing.T) {
	test.GenericCache(t, newTestCache)
}

// FIXME: Tests for the various corner cases / failure cases of boltDBCache should be added here.

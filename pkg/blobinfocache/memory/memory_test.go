package memory

import (
	"testing"

	"github.com/containers/image/v5/internal/blobinfocache"
	"github.com/containers/image/v5/pkg/blobinfocache/internal/test"
)

var _ blobinfocache.BlobInfoCache2 = &cache{}

func newTestCache(t *testing.T) (blobinfocache.BlobInfoCache2, func(t *testing.T)) {
	return new2(), func(t *testing.T) {}
}

func TestNew(t *testing.T) {
	test.GenericCache(t, newTestCache)
}

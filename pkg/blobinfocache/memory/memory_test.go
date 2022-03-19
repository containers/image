package memory

import (
	"testing"

	"github.com/containers/image/v5/internal/blobinfocache"
	"github.com/containers/image/v5/pkg/blobinfocache/internal/test"
)

var _ blobinfocache.BlobInfoCache2 = &cache{}

func newTestCache(t *testing.T) blobinfocache.BlobInfoCache2 {
	return new2()
}

func TestNew(t *testing.T) {
	test.GenericCache(t, newTestCache)
}

package copy

import (
	"testing"

	digest "github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
)

func TestPrepareCopyInstances(t *testing.T) {
	// Test CopyAllImages
	sourceInstances := []digest.Digest{
		digest.Digest("sha256:5b0bcabd1ed22e9fb1310cf6c2dec7cdef19f0ad69efa1f392e94a4333501270"),
		digest.Digest("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		digest.Digest("sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"),
	}

	instancesToCopy := prepareInstanceCopies(sourceInstances, &Options{})
	compare := []instanceCopy{}
	for _, instance := range sourceInstances {
		compare = append(compare, instanceCopy{op: instanceCopyCopy,
			sourceDigest: instance})
	}
	assert.Equal(t, instancesToCopy, compare)

	// Test with CopySpecific Images
	copyOnly := []digest.Digest{
		digest.Digest("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
	}
	instancesToCopy = prepareInstanceCopies(sourceInstances, &Options{
		Instances:          copyOnly,
		ImageListSelection: CopySpecificImages})
	assert.Equal(t, instancesToCopy, []instanceCopy{{
		op:           instanceCopyCopy,
		sourceDigest: copyOnly[0]}})
}

package copy

import (
	"os"
	"path/filepath"
	"testing"

	internalManifest "github.com/containers/image/v5/internal/manifest"
	digest "github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test `instanceCopyCopy` cases.
func TestPrepareCopyInstancesforInstanceCopyCopy(t *testing.T) {
	validManifest, err := os.ReadFile(filepath.Join("..", "internal", "manifest", "testdata", "oci1.index.zstd-selection.json"))
	require.NoError(t, err)
	list, err := internalManifest.ListFromBlob(validManifest, internalManifest.GuessMIMEType(validManifest))
	require.NoError(t, err)

	// Test CopyAllImages
	sourceInstances := []digest.Digest{
		digest.Digest("sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		digest.Digest("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		digest.Digest("sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"),
	}

	instancesToCopy, err := prepareInstanceCopies(list, sourceInstances, &Options{})
	require.NoError(t, err)
	compare := []instanceCopy{}

	for _, instance := range sourceInstances {
		compare = append(compare, instanceCopy{op: instanceCopyCopy,
			sourceDigest: instance})
	}
	assert.Equal(t, instancesToCopy, compare)

	// Test CopySpecificImages where selected instance is sourceInstances[1]
	instancesToCopy, err = prepareInstanceCopies(list, sourceInstances, &Options{Instances: []digest.Digest{sourceInstances[1]}, ImageListSelection: CopySpecificImages})
	require.NoError(t, err)
	compare = []instanceCopy{{op: instanceCopyCopy,
		sourceDigest: sourceInstances[1]}}
	assert.Equal(t, instancesToCopy, compare)
}

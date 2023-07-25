package copy

import (
	"os"
	"path/filepath"
	"testing"

	internalManifest "github.com/containers/image/v5/internal/manifest"
	"github.com/containers/image/v5/pkg/compression"
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

// Test `instanceCopyClone` cases.
func TestPrepareCopyInstancesforInstanceCopyClone(t *testing.T) {
	validManifest, err := os.ReadFile(filepath.Join("..", "internal", "manifest", "testdata", "oci1.index.zstd-selection.json"))
	require.NoError(t, err)
	list, err := internalManifest.ListFromBlob(validManifest, internalManifest.GuessMIMEType(validManifest))
	require.NoError(t, err)

	// Prepare option for `instanceCopyClone` case.
	ensureCompressionVariantsExist := []OptionCompressionVariant{{Algorithm: compression.Zstd}}

	sourceInstances := []digest.Digest{
		digest.Digest("sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		digest.Digest("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		digest.Digest("sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"),
	}

	// CopySpecificImage must fail with error
	_, err = prepareInstanceCopies(list, sourceInstances, &Options{EnsureCompressionVariantsExist: ensureCompressionVariantsExist,
		Instances:          []digest.Digest{sourceInstances[1]},
		ImageListSelection: CopySpecificImages})
	require.EqualError(t, err, "EnsureCompressionVariantsExist is not implemented for CopySpecificImages")

	// Test copying all images with replication
	instancesToCopy, err := prepareInstanceCopies(list, sourceInstances, &Options{EnsureCompressionVariantsExist: ensureCompressionVariantsExist})
	require.NoError(t, err)

	// Following test ensures
	// * Still copy gzip variants if they exist in the original
	// * Not create new Zstd variants if they exist in the original.

	// We crated a list of three instances `sourceInstances` and since in oci1.index.zstd-selection.json
	// amd64 already has a zstd instance i.e sourceInstance[1] so it should not create replication for
	// `sourceInstance[0]` and `sourceInstance[1]` but should do it for `sourceInstance[2]` for `arm64`
	// and still copy `sourceInstance[2]`.
	expectedResponse := []simplerInstanceCopy{}
	for _, instance := range sourceInstances {
		// If its `arm64` and sourceDigest[2] , expect a clone to happen
		if instance == sourceInstances[2] {
			expectedResponse = append(expectedResponse, simplerInstanceCopy{op: instanceCopyClone, sourceDigest: instance, cloneCompressionVariant: "zstd", clonePlatform: "arm64-linux-"})
		}
		expectedResponse = append(expectedResponse, simplerInstanceCopy{op: instanceCopyCopy,
			sourceDigest: instance})
	}
	actualResponse := convertInstanceCopyToSimplerInstanceCopy(instancesToCopy)
	assert.Equal(t, expectedResponse, actualResponse)

	// Test option with multiple copy request for same compression format
	// above expection should stay same, if out ensureCompressionVariantsExist requests zstd twice
	ensureCompressionVariantsExist = []OptionCompressionVariant{{Algorithm: compression.Zstd}, {Algorithm: compression.Zstd}}
	instancesToCopy, err = prepareInstanceCopies(list, sourceInstances, &Options{EnsureCompressionVariantsExist: ensureCompressionVariantsExist})
	require.NoError(t, err)
	expectedResponse = []simplerInstanceCopy{}
	for _, instance := range sourceInstances {
		// If its `arm64` and sourceDigest[2] , expect a clone to happen
		if instance == sourceInstances[2] {
			expectedResponse = append(expectedResponse, simplerInstanceCopy{op: instanceCopyClone, sourceDigest: instance, cloneCompressionVariant: "zstd", clonePlatform: "arm64-linux-"})
		}
		expectedResponse = append(expectedResponse, simplerInstanceCopy{op: instanceCopyCopy,
			sourceDigest: instance})
	}
	actualResponse = convertInstanceCopyToSimplerInstanceCopy(instancesToCopy)
	assert.Equal(t, expectedResponse, actualResponse)

	// Add same instance twice but clone must appear only once.
	ensureCompressionVariantsExist = []OptionCompressionVariant{{Algorithm: compression.Zstd}}
	sourceInstances = []digest.Digest{
		digest.Digest("sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"),
		digest.Digest("sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"),
	}
	instancesToCopy, err = prepareInstanceCopies(list, sourceInstances, &Options{EnsureCompressionVariantsExist: ensureCompressionVariantsExist})
	require.NoError(t, err)
	// two copies but clone should happen only once
	numberOfCopyClone := 0
	for _, instance := range instancesToCopy {
		if instance.op == instanceCopyClone {
			numberOfCopyClone++
		}
	}
	assert.Equal(t, 1, numberOfCopyClone)
}

// simpler version of `instanceCopy` for testing where fields are string
// instead of pointer
type simplerInstanceCopy struct {
	op           instanceCopyKind
	sourceDigest digest.Digest

	// Fields which can be used by callers when operation
	// is `instanceCopyClone`
	cloneCompressionVariant string
	clonePlatform           string
	cloneAnnotations        map[string]string
}

func convertInstanceCopyToSimplerInstanceCopy(copies []instanceCopy) []simplerInstanceCopy {
	res := []simplerInstanceCopy{}
	for _, instance := range copies {
		compression := ""
		platform := ""
		compression = instance.cloneCompressionVariant.Algorithm.Name()
		if instance.clonePlatform != nil {
			platform = instance.clonePlatform.Architecture + "-" + instance.clonePlatform.OS + "-" + instance.clonePlatform.Variant
		}
		res = append(res, simplerInstanceCopy{
			op:                      instance.op,
			sourceDigest:            instance.sourceDigest,
			cloneCompressionVariant: compression,
			clonePlatform:           platform,
			cloneAnnotations:        instance.cloneAnnotations,
		})
	}
	return res
}

package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/opencontainers/go-digest"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchema2ListPublicFromManifest(t *testing.T) {
	validManifest, err := os.ReadFile(filepath.Join("testdata", "v2list.manifest.json"))
	require.NoError(t, err)

	parser := func(m []byte) error {
		_, err := Schema2ListPublicFromManifest(m)
		return err
	}
	// Schema mismatch is rejected
	testManifestFixturesAreRejected(t, parser, []string{
		"schema2-to-schema1-by-docker.json",
		"v2s2.manifest.json",
		"ociv1.manifest.json",
		// Not "ociv1.image.index.json" yet, without validating mediaType the two are too similar to tell the difference.
	})
	// Extra fields are rejected
	testValidManifestWithExtraFieldsIsRejected(t, parser, validManifest, []string{"config", "fsLayers", "history", "layers"})
}

func TestSchema2ListEditInstances(t *testing.T) {
	validManifest, err := os.ReadFile(filepath.Join("testdata", "v2list.manifest.json"))
	require.NoError(t, err)
	list, err := ListFromBlob(validManifest, GuessMIMEType(validManifest))
	require.NoError(t, err)

	expectedDigests := list.Instances()
	editInstances := []ListEdit{}
	editInstances = append(editInstances, ListEdit{
		UpdateOldDigest: list.Instances()[0],
		UpdateDigest:    "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		UpdateSize:      32,
		UpdateMediaType: "something",
		ListOperation:   ListOpUpdate})
	err = list.EditInstances(editInstances)
	require.NoError(t, err)

	expectedDigests[0] = editInstances[0].UpdateDigest
	// order of old elements must remain same.
	assert.Equal(t, list.Instances(), expectedDigests)

	instance, err := list.Instance(list.Instances()[0])
	require.NoError(t, err)
	assert.Equal(t, "something", instance.MediaType)
	assert.Equal(t, int64(32), instance.Size)
	// platform must match with instance platform set in `v2list.manifest.json` for the first instance
	assert.Equal(t, &imgspecv1.Platform{Architecture: "ppc64le", OS: "linux", OSVersion: "", OSFeatures: []string(nil), Variant: ""}, instance.Platform)

	// Create a fresh list
	list, err = ListFromBlob(validManifest, GuessMIMEType(validManifest))
	require.NoError(t, err)
	originalListOrder := list.Instances()

	editInstances = []ListEdit{}
	editInstances = append(editInstances, ListEdit{
		AddDigest:     "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		AddSize:       32,
		AddMediaType:  "application/vnd.oci.image.manifest.v1+json",
		AddPlatform:   &imgspecv1.Platform{Architecture: "amd64", OS: "linux", OSFeatures: []string{"sse4"}},
		ListOperation: ListOpAdd})
	editInstances = append(editInstances, ListEdit{
		AddDigest:     "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		AddSize:       32,
		AddMediaType:  "application/vnd.oci.image.manifest.v1+json",
		AddPlatform:   &imgspecv1.Platform{Architecture: "amd64", OS: "linux", OSFeatures: []string{"sse4"}},
		ListOperation: ListOpAdd})
	err = list.EditInstances(editInstances)
	require.NoError(t, err)

	// Add new elements to the end of old list to maintain order
	originalListOrder = append(originalListOrder, digest.Digest("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
	originalListOrder = append(originalListOrder, digest.Digest("sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"))
	// Verify order
	assert.Equal(t, list.Instances(), originalListOrder)
}

func TestSchema2ListFromManifest(t *testing.T) {
	validManifest, err := os.ReadFile(filepath.Join("testdata", "v2list.manifest.json"))
	require.NoError(t, err)

	parser := func(m []byte) error {
		_, err := Schema2ListFromManifest(m)
		return err
	}
	// Schema mismatch is rejected
	testManifestFixturesAreRejected(t, parser, []string{
		"schema2-to-schema1-by-docker.json",
		"v2s2.manifest.json",
		"ociv1.manifest.json",
		// Not "ociv1.image.index.json" yet, without validating mediaType the two are too similar to tell the difference.
	})
	// Extra fields are rejected
	testValidManifestWithExtraFieldsIsRejected(t, parser, validManifest, []string{"config", "fsLayers", "history", "layers"})
}

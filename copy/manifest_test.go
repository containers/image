package copy

import (
	"errors"
	"testing"

	"github.com/containers/image/manifest"
	"github.com/containers/image/types"
	"github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeImageSource is an implementation of types.Image which only returns itself as a MIME type in Manifest
// except that "" means “reading the manifest should fail”
type fakeImageSource string

func (f fakeImageSource) Reference() types.ImageReference {
	panic("Unexpected call to a mock function")
}
func (f fakeImageSource) Close() error {
	panic("Unexpected call to a mock function")
}
func (f fakeImageSource) Manifest() ([]byte, string, error) {
	if string(f) == "" {
		return nil, "", errors.New("Manifest() directed to fail")
	}
	return nil, string(f), nil
}
func (f fakeImageSource) Signatures() ([][]byte, error) {
	panic("Unexpected call to a mock function")
}
func (f fakeImageSource) ConfigInfo() types.BlobInfo {
	panic("Unexpected call to a mock function")
}
func (f fakeImageSource) ConfigBlob() ([]byte, error) {
	panic("Unexpected call to a mock function")
}
func (f fakeImageSource) OCIConfig() (*v1.Image, error) {
	panic("Unexpected call to a mock function")
}
func (f fakeImageSource) LayerInfos() []types.BlobInfo {
	panic("Unexpected call to a mock function")
}
func (f fakeImageSource) Inspect() (*types.ImageInspectInfo, error) {
	panic("Unexpected call to a mock function")
}
func (f fakeImageSource) UpdatedImageNeedsLayerDiffIDs(options types.ManifestUpdateOptions) bool {
	panic("Unexpected call to a mock function")
}
func (f fakeImageSource) UpdatedImage(options types.ManifestUpdateOptions) (types.Image, error) {
	panic("Unexpected call to a mock function")
}
func (f fakeImageSource) IsMultiImage() bool {
	panic("Unexpected call to a mock function")
}
func (f fakeImageSource) Size() (int64, error) {
	panic("Unexpected call to a mock function")
}

func TestDetermineManifestConversion(t *testing.T) {
	supportS1S2OCI := []string{
		v1.MediaTypeImageManifest,
		manifest.DockerV2Schema2MediaType,
		manifest.DockerV2Schema1SignedMediaType,
		manifest.DockerV2Schema1MediaType,
	}
	supportS1OCI := []string{
		v1.MediaTypeImageManifest,
		manifest.DockerV2Schema1SignedMediaType,
		manifest.DockerV2Schema1MediaType,
	}
	supportS1S2 := []string{
		manifest.DockerV2Schema2MediaType,
		manifest.DockerV2Schema1SignedMediaType,
		manifest.DockerV2Schema1MediaType,
	}
	supportOnlyS1 := []string{
		manifest.DockerV2Schema1SignedMediaType,
		manifest.DockerV2Schema1MediaType,
	}

	cases := []struct {
		description    string
		sourceType     string
		destTypes      []string
		expectedUpdate string
	}{
		// Destination accepts anything — no conversion necessary
		{"s1→anything", manifest.DockerV2Schema1SignedMediaType, nil, ""},
		{"s2→anything", manifest.DockerV2Schema2MediaType, nil, ""},
		// Destination accepts the unmodified original
		{"s1→s1s2", manifest.DockerV2Schema1SignedMediaType, supportS1S2, ""},
		{"s2→s1s2", manifest.DockerV2Schema2MediaType, supportS1S2, ""},
		{"s1→s1", manifest.DockerV2Schema1SignedMediaType, supportOnlyS1, ""},
		// Conversion necessary, a preferred format is acceptable
		{"s2→s1", manifest.DockerV2Schema2MediaType, supportOnlyS1, manifest.DockerV2Schema1SignedMediaType},
		// Conversion necessary, a preferred format is not acceptable
		{"s2→OCI", manifest.DockerV2Schema2MediaType, []string{v1.MediaTypeImageManifest}, v1.MediaTypeImageManifest},
		// Conversion necessary, try the preferred formats in order.
		{"special→s2", "this needs conversion", supportS1S2OCI, manifest.DockerV2Schema2MediaType},
		{"special→s1", "this needs conversion", supportS1OCI, manifest.DockerV2Schema1SignedMediaType},
		{"special→OCI", "this needs conversion", []string{v1.MediaTypeImageManifest, "other options", "with lower priority"}, v1.MediaTypeImageManifest},
	}

	for _, c := range cases {
		src := fakeImageSource(c.sourceType)
		mu := types.ManifestUpdateOptions{}
		preferredMIMEType, err := determineManifestConversion(&mu, src, c.destTypes, true)
		require.NoError(t, err, c.description)
		assert.Equal(t, c.expectedUpdate, mu.ManifestMIMEType, c.description)
		if c.expectedUpdate == "" {
			assert.Equal(t, c.sourceType, preferredMIMEType, c.description)
		} else {
			assert.Equal(t, c.expectedUpdate, preferredMIMEType, c.description)
		}
	}

	// Whatever the input is, with !canModifyManifest we return "keep the original as is"
	for _, c := range cases {
		src := fakeImageSource(c.sourceType)
		mu := types.ManifestUpdateOptions{}
		preferredMIMEType, err := determineManifestConversion(&mu, src, c.destTypes, false)
		require.NoError(t, err, c.description)
		assert.Equal(t, "", mu.ManifestMIMEType, c.description)
		assert.Equal(t, c.sourceType, preferredMIMEType, c.description)
	}

	// Error reading the manifest — smoke test only.
	mu := types.ManifestUpdateOptions{}
	_, err := determineManifestConversion(&mu, fakeImageSource(""), supportS1S2, true)
	assert.Error(t, err)
}

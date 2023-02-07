package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/containers/image/v5/types"
	"github.com/opencontainers/go-digest"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func pare(m List) {
	if impl, ok := m.(*OCI1Index); ok {
		impl.Annotations = nil
	}
	if impl, ok := m.(*Schema2List); ok {
		for i := range impl.Manifests {
			impl.Manifests[i].Platform.Features = nil
		}
	}
}

func TestParseLists(t *testing.T) {
	cases := []struct {
		path     string
		mimeType string
	}{
		{"ociv1.image.index.json", imgspecv1.MediaTypeImageIndex},
		{"v2list.manifest.json", DockerV2ListMediaType},
	}
	for _, c := range cases {
		manifest, err := os.ReadFile(filepath.Join("testdata", c.path))
		require.NoError(t, err, "error reading file %q", filepath.Join("testdata", c.path))
		assert.Equal(t, GuessMIMEType(manifest), c.mimeType)

		// c/image/manifest.TestParseLists verifies that FromBlob refuses to parse the manifest list

		m, err := ListFromBlob(manifest, c.mimeType)
		require.NoError(t, err, "manifest list %q  should parse as list types", c.path)
		assert.Equal(t, m.MIMEType(), c.mimeType, "manifest %q is not of the expected MIME type", c.path)

		clone := m.Clone()
		assert.Equal(t, clone, m, "manifest %q is missing some fields after being cloned", c.path)

		pare(m)

		index, err := m.ConvertToMIMEType(imgspecv1.MediaTypeImageIndex)
		require.NoError(t, err, "error converting %q to an OCI1Index", c.path)

		list, err := m.ConvertToMIMEType(DockerV2ListMediaType)
		require.NoError(t, err, "error converting %q to an Schema2List", c.path)

		index2, err := list.ConvertToMIMEType(imgspecv1.MediaTypeImageIndex)
		require.NoError(t, err)
		assert.Equal(t, index, index2, "index %q lost data in conversion", c.path)

		list2, err := index.ConvertToMIMEType(DockerV2ListMediaType)
		require.NoError(t, err)
		assert.Equal(t, list, list2, "list %q lost data in conversion", c.path)
	}
}

func TestChooseInstance(t *testing.T) {
	type expectedMatch struct {
		arch, variant  string
		instanceDigest digest.Digest
	}
	for _, manifestList := range []struct {
		listFile           string
		matchedInstances   []expectedMatch
		unmatchedInstances []string
	}{
		{
			listFile: "schema2list.json",
			matchedInstances: []expectedMatch{
				{"amd64", "", "sha256:030fcb92e1487b18c974784dcc110a93147c9fc402188370fbfd17efabffc6af"},
				{"s390x", "", "sha256:e5aa1b0a24620228b75382997a0977f609b3ca3a95533dafdef84c74cc8df642"},
				{"arm", "v7", "sha256:b5dbad4bdb4444d919294afe49a095c23e86782f98cdf0aa286198ddb814b50b"},
				{"arm64", "", "sha256:dc472a59fb006797aa2a6bfb54cc9c57959bb0a6d11fadaa608df8c16dea39cf"},
			},
			unmatchedInstances: []string{
				"unmatched",
			},
		},
		{ // Focus on ARM variant field testing
			listFile: "schema2list-variants.json",
			matchedInstances: []expectedMatch{
				{"amd64", "", "sha256:59eec8837a4d942cc19a52b8c09ea75121acc38114a2c68b98983ce9356b8610"},
				{"arm", "v7", "sha256:f365626a556e58189fc21d099fc64603db0f440bff07f77c740989515c544a39"},
				{"arm", "v6", "sha256:f365626a556e58189fc21d099fc64603db0f440bff07f77c740989515c544a39"},
				{"arm", "v5", "sha256:c84b0a3a07b628bc4d62e5047d0f8dff80f7c00979e1e28a821a033ecda8fe53"},
				{"arm", "", "sha256:c84b0a3a07b628bc4d62e5047d0f8dff80f7c00979e1e28a821a033ecda8fe53"},
				{"arm", "unrecognized-present", "sha256:bcf9771c0b505e68c65440474179592ffdfa98790eb54ffbf129969c5e429990"},
				{"arm", "unrecognized-not-present", "sha256:c84b0a3a07b628bc4d62e5047d0f8dff80f7c00979e1e28a821a033ecda8fe53"},
			},
			unmatchedInstances: []string{
				"unmatched",
			},
		},
		{
			listFile: "oci1index.json",
			matchedInstances: []expectedMatch{
				{"amd64", "", "sha256:5b0bcabd1ed22e9fb1310cf6c2dec7cdef19f0ad69efa1f392e94a4333501270"},
				{"ppc64le", "", "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f"},
			},
			unmatchedInstances: []string{
				"unmatched",
			},
		},
	} {
		rawManifest, err := os.ReadFile(filepath.Join("..", "..", "internal", "image", "fixtures", manifestList.listFile))
		require.NoError(t, err)
		list, err := ListPublicFromBlob(rawManifest, GuessMIMEType(rawManifest))
		require.NoError(t, err)
		// Match found
		for _, match := range manifestList.matchedInstances {
			testName := fmt.Sprintf("%s %q+%q", manifestList.listFile, match.arch, match.variant)
			digest, err := list.ChooseInstance(&types.SystemContext{
				ArchitectureChoice: match.arch,
				VariantChoice:      match.variant,
				OSChoice:           "linux",
			})
			require.NoError(t, err, testName)
			assert.Equal(t, match.instanceDigest, digest, testName)
		}
		// Not found
		for _, arch := range manifestList.unmatchedInstances {
			_, err := list.ChooseInstance(&types.SystemContext{
				ArchitectureChoice: arch,
				OSChoice:           "linux",
			})
			assert.Error(t, err)
		}
	}
}

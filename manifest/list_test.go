package manifest

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/containers/image/v5/types"
	digest "github.com/opencontainers/go-digest"
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
		manifest, err := ioutil.ReadFile(filepath.Join("fixtures", c.path))
		require.NoError(t, err, "error reading file %q", filepath.Join("fixtures", c.path))
		assert.Equal(t, GuessMIMEType(manifest), c.mimeType)

		_, err = FromBlob(manifest, c.mimeType)
		require.Error(t, err, "manifest list %q should not parse as single images", c.path)

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
	for _, manifestList := range []struct {
		listFile           string
		matchedInstances   map[string]digest.Digest
		unmatchedInstances []string
	}{
		{
			listFile: "schema2list.json",
			matchedInstances: map[string]digest.Digest{
				"amd64": "sha256:030fcb92e1487b18c974784dcc110a93147c9fc402188370fbfd17efabffc6af",
				"s390x": "sha256:e5aa1b0a24620228b75382997a0977f609b3ca3a95533dafdef84c74cc8df642",
				"arm":   "sha256:b5dbad4bdb4444d919294afe49a095c23e86782f98cdf0aa286198ddb814b50b",
			},
			unmatchedInstances: []string{
				"unmatched",
			},
		},
		{
			listFile: "oci1index.json",
			matchedInstances: map[string]digest.Digest{
				"amd64":   "sha256:5b0bcabd1ed22e9fb1310cf6c2dec7cdef19f0ad69efa1f392e94a4333501270",
				"ppc64le": "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f",
			},
			unmatchedInstances: []string{
				"unmatched",
			},
		},
	} {
		man, err := ioutil.ReadFile(filepath.Join("..", "image", "fixtures", manifestList.listFile))
		require.NoError(t, err)
		rawManifest := man
		list, err := ListFromBlob(rawManifest, GuessMIMEType(rawManifest))
		require.NoError(t, err)
		// Match found
		for arch, expected := range manifestList.matchedInstances {
			digest, err := list.ChooseInstance(&types.SystemContext{
				ArchitectureChoice: arch,
				OSChoice:           "linux",
				VariantChoice:      "v6",
			})
			require.NoError(t, err, arch)
			assert.Equal(t, expected, digest)
		}
		// Not found
		for _, arch := range manifestList.unmatchedInstances {
			_, err := list.ChooseInstance(&types.SystemContext{
				ArchitectureChoice: arch,
				OSChoice:           "linux",
				VariantChoice:      "v6",
			})
			assert.Error(t, err)
		}
	}
}

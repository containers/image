package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/containers/image/v5/pkg/compression"
	"github.com/containers/image/v5/types"
	"github.com/opencontainers/go-digest"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOCI1IndexPublicFromManifest(t *testing.T) {
	validManifest, err := os.ReadFile(filepath.Join("testdata", "ociv1.image.index.json"))
	require.NoError(t, err)

	parser := func(m []byte) error {
		_, err := OCI1IndexPublicFromManifest(m)
		return err
	}
	// Schema mismatch is rejected
	testManifestFixturesAreRejected(t, parser, []string{
		"schema2-to-schema1-by-docker.json",
		"v2s2.manifest.json",
		// Not "v2list.manifest.json" yet, without mediaType the two are too similar to tell the difference.
		"ociv1.manifest.json",
	})
	// Extra fields are rejected
	testValidManifestWithExtraFieldsIsRejected(t, parser, validManifest, []string{"config", "fsLayers", "history", "layers"})
}

func TestOCI1IndexFromManifest(t *testing.T) {
	validManifest, err := os.ReadFile(filepath.Join("testdata", "ociv1.image.index.json"))
	require.NoError(t, err)

	parser := func(m []byte) error {
		_, err := OCI1IndexFromManifest(m)
		return err
	}
	// Schema mismatch is rejected
	testManifestFixturesAreRejected(t, parser, []string{
		"schema2-to-schema1-by-docker.json",
		"v2s2.manifest.json",
		// Not "v2list.manifest.json" yet, without mediaType the two are too similar to tell the difference.
		"ociv1.manifest.json",
	})
	// Extra fields are rejected
	testValidManifestWithExtraFieldsIsRejected(t, parser, validManifest, []string{"config", "fsLayers", "history", "layers"})
}

func TestOCI1EditInstances(t *testing.T) {
	validManifest, err := os.ReadFile(filepath.Join("testdata", "ociv1.image.index.json"))
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

	// Create a fresh list
	list, err = ListFromBlob(validManifest, GuessMIMEType(validManifest))
	require.NoError(t, err)

	// Verfiy correct zstd sorting
	editInstances = []ListEdit{}
	annotations := map[string]string{"io.github.containers.compression.zstd": "true"}
	// without zstd
	editInstances = append(editInstances, ListEdit{
		AddDigest:     "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		AddSize:       32,
		AddMediaType:  "application/vnd.oci.image.manifest.v1+json",
		AddPlatform:   &imgspecv1.Platform{Architecture: "amd64", OS: "linux", OSFeatures: []string{"sse4"}},
		ListOperation: ListOpAdd})
	// with zstd
	editInstances = append(editInstances, ListEdit{
		AddDigest:      "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		AddSize:        32,
		AddMediaType:   "application/vnd.oci.image.manifest.v1+json",
		AddPlatform:    &imgspecv1.Platform{Architecture: "amd64", OS: "linux", OSFeatures: []string{"sse4"}},
		AddAnnotations: annotations,
		ListOperation:  ListOpAdd})
	// with zstd but with compression, annotation must be added automatically
	editInstances = append(editInstances, ListEdit{
		AddDigest:                "sha256:hhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhh",
		AddSize:                  32,
		AddMediaType:             "application/vnd.oci.image.manifest.v1+json",
		AddPlatform:              &imgspecv1.Platform{Architecture: "amd64", OS: "linux", OSFeatures: []string{"sse4"}},
		AddCompressionAlgorithms: []compression.Algorithm{compression.Zstd},
		AddAnnotations:           map[string]string{},
		ListOperation:            ListOpAdd})
	// with zstd but with compression, annotation must be added automatically and AddAnnotations is unset
	editInstances = append(editInstances, ListEdit{
		AddDigest:                "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		AddSize:                  32,
		AddMediaType:             "application/vnd.oci.image.manifest.v1+json",
		AddPlatform:              &imgspecv1.Platform{Architecture: "amd64", OS: "linux", OSFeatures: []string{"sse4"}},
		AddCompressionAlgorithms: []compression.Algorithm{compression.Zstd},
		ListOperation:            ListOpAdd})
	// without zstd
	editInstances = append(editInstances, ListEdit{
		AddDigest:     "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		AddSize:       32,
		AddMediaType:  "application/vnd.oci.image.manifest.v1+json",
		AddPlatform:   &imgspecv1.Platform{Architecture: "amd64", OS: "linux", OSFeatures: []string{"sse4"}},
		ListOperation: ListOpAdd})
	err = list.EditInstances(editInstances)
	require.NoError(t, err)

	// Zstd should be kept on lowest priority as compared to the default gzip ones and order of prior elements must be preserved.
	assert.Equal(t, list.Instances(), []digest.Digest{digest.Digest("sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f"), digest.Digest("sha256:5b0bcabd1ed22e9fb1310cf6c2dec7cdef19f0ad69efa1f392e94a4333501270"), digest.Digest("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), digest.Digest("sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"), digest.Digest("sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"), digest.Digest("sha256:hhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhh"), digest.Digest("sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")})

	// Update list and remove zstd annotation from existing instance, and verify if resorting works
	editInstances = []ListEdit{}
	editInstances = append(editInstances, ListEdit{
		UpdateOldDigest:         digest.Digest("sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"),
		UpdateDigest:            "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		UpdateSize:              32,
		UpdateMediaType:         "application/vnd.oci.image.manifest.v1+json",
		UpdateAffectAnnotations: true,
		UpdateAnnotations:       map[string]string{},
		ListOperation:           ListOpUpdate})
	err = list.EditInstances(editInstances)
	require.NoError(t, err)
	// Digest `ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff` should be re-ordered on update.
	assert.Equal(t, list.Instances(), []digest.Digest{digest.Digest("sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f"), digest.Digest("sha256:5b0bcabd1ed22e9fb1310cf6c2dec7cdef19f0ad69efa1f392e94a4333501270"), digest.Digest("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), digest.Digest("sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"), digest.Digest("sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"), digest.Digest("sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"), digest.Digest("sha256:hhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhh")})

}

func TestOCI1IndexChooseInstanceByCompression(t *testing.T) {
	type expectedMatch struct {
		arch, variant  string
		instanceDigest digest.Digest
		preferGzip     bool
	}
	for _, manifestList := range []struct {
		listFile           string
		matchedInstances   []expectedMatch
		unmatchedInstances []string
	}{
		{
			listFile: "oci1.index.zstd-selection.json",
			matchedInstances: []expectedMatch{
				// out of gzip and zstd in amd64 select the first zstd image
				{"amd64", "", "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", false},
				// out of multiple gzip in arm64 select the first one to ensure original logic is prevented
				{"arm64", "", "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", false},
				// select a signle gzip s390x image
				{"s390x", "", "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", false},
				// out of gzip and zstd in amd64 select the first gzip image
				{"amd64", "", "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true},
				// out of multiple gzip in arm64 select the first one to ensure original logic is prevented
				{"arm64", "", "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", true},
				// select a signle gzip s390x image
				{"s390x", "", "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", true},
			},
			unmatchedInstances: []string{
				"unmatched",
			},
		},
		{ // Focus on ARM variant field testing
			listFile: "ocilist-variants.json",
			matchedInstances: []expectedMatch{
				{"amd64", "", "sha256:59eec8837a4d942cc19a52b8c09ea75121acc38114a2c68b98983ce9356b8610", false},
				{"arm", "v7", "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", false},
				{"arm", "v6", "sha256:f365626a556e58189fc21d099fc64603db0f440bff07f77c740989515c544a39", false},
				{"arm", "v5", "sha256:c84b0a3a07b628bc4d62e5047d0f8dff80f7c00979e1e28a821a033ecda8fe53", false},
				{"arm", "", "sha256:c84b0a3a07b628bc4d62e5047d0f8dff80f7c00979e1e28a821a033ecda8fe53", false},
				{"arm", "unrecognized-present", "sha256:bcf9771c0b505e68c65440474179592ffdfa98790eb54ffbf129969c5e429990", false},
				{"arm", "unrecognized-not-present", "sha256:c84b0a3a07b628bc4d62e5047d0f8dff80f7c00979e1e28a821a033ecda8fe53", false},
				// preferGzip true
				{"amd64", "", "sha256:59eec8837a4d942cc19a52b8c09ea75121acc38114a2c68b98983ce9356b8610", true},
				{"arm", "v7", "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", true},
				{"arm", "v6", "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", true},
				{"arm", "v5", "sha256:c84b0a3a07b628bc4d62e5047d0f8dff80f7c00979e1e28a821a033ecda8fe53", true},
				{"arm", "", "sha256:c84b0a3a07b628bc4d62e5047d0f8dff80f7c00979e1e28a821a033ecda8fe53", true},
				{"arm", "unrecognized-present", "sha256:bcf9771c0b505e68c65440474179592ffdfa98790eb54ffbf129969c5e429990", true},
				{"arm", "unrecognized-not-present", "sha256:c84b0a3a07b628bc4d62e5047d0f8dff80f7c00979e1e28a821a033ecda8fe53", true},
			},
			unmatchedInstances: []string{
				"unmatched",
			},
		},
		{
			listFile: "oci1.index.zstd-selection2.json",
			// out of list where first instance is gzip , select the first occurance of zstd out of many
			matchedInstances: []expectedMatch{
				{"amd64", "", "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", false},
				{"amd64", "", "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true},
				// must return first gzip even if the first entry is zstd
				{"arm64", "", "sha256:6dc14a60d2ba724646cfbf5fccbb9a618a5978a64a352e060b17caf5e005da9d", true},
				// must return first zstd even if the first entry for same platform is gzip
				{"arm64", "", "sha256:1c98002b30a71b08ab175915ce7c8fb8da9e9b502ae082d6f0c572bac9dee324", false},
				// must return first zstd instance with no platform
				{"matchesImageWithNoPlatform", "", "sha256:f2f5f52a2cf2c51d4cac6df0545f751c0adc3f3427eb47c59fcb32894503e18f", false},
				// must return first gzip instance with no platform
				{"matchesImageWithNoPlatform", "", "sha256:c76757bb6006babdd8464dbf2f1157fdfa6fead0bc6f84f15816a32d6f68f706", true},
			},
		},
		{
			listFile: "oci1index.json",
			matchedInstances: []expectedMatch{
				{"amd64", "", "sha256:5b0bcabd1ed22e9fb1310cf6c2dec7cdef19f0ad69efa1f392e94a4333501270", false},
				{"ppc64le", "", "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f", false},
			},
			unmatchedInstances: []string{
				"unmatched",
			},
		},
	} {
		rawManifest, err := os.ReadFile(filepath.Join("testdata", manifestList.listFile))
		require.NoError(t, err)
		list, err := ListFromBlob(rawManifest, GuessMIMEType(rawManifest))
		require.NoError(t, err)
		for _, match := range manifestList.matchedInstances {
			testName := fmt.Sprintf("%s %q+%q", manifestList.listFile, match.arch, match.variant)
			digest, err := list.ChooseInstanceByCompression(&types.SystemContext{
				ArchitectureChoice: match.arch,
				VariantChoice:      match.variant,
				OSChoice:           "linux",
			}, types.NewOptionalBool(match.preferGzip))
			require.NoError(t, err, testName)
			assert.Equal(t, match.instanceDigest, digest, testName)
		}
		for _, arch := range manifestList.unmatchedInstances {
			_, err := list.ChooseInstanceByCompression(&types.SystemContext{
				ArchitectureChoice: arch,
				OSChoice:           "linux",
			}, types.NewOptionalBool(false))
			assert.Error(t, err)
		}
	}
}

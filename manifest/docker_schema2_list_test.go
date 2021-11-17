package manifest

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSchema2ListFromManifest(t *testing.T) {
	validManifest, err := ioutil.ReadFile(filepath.Join("fixtures", "v2list.manifest.json"))
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

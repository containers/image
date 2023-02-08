package manifest

import (
	"os"
	"path/filepath"
	"testing"

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

package manifest

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateUnambiguousManifestFormat(t *testing.T) {
	const allAllowedFields = AllowedFieldFirstUnusedBit - 1
	const mt = "text/plain" // Just some MIME type that shows up in error messages

	type test struct {
		manifest string
		allowed  AllowedManifestFields
	}

	// Smoke tests: Success
	for _, c := range []test{
		{"{}", allAllowedFields},
		{"{}", 0},
	} {
		err := ValidateUnambiguousManifestFormat([]byte(c.manifest), mt, c.allowed)
		assert.NoError(t, err, c)
	}
	// Smoke tests: Failure
	for _, c := range []test{
		{"{}", AllowedFieldFirstUnusedBit}, // Invalid "allowed"
		{"@", allAllowedFields},            // Invalid JSON
	} {
		err := ValidateUnambiguousManifestFormat([]byte(c.manifest), mt, c.allowed)
		assert.Error(t, err, c)
	}

	fields := map[AllowedManifestFields]string{
		AllowedFieldConfig:    "config",
		AllowedFieldFSLayers:  "fsLayers",
		AllowedFieldHistory:   "history",
		AllowedFieldLayers:    "layers",
		AllowedFieldManifests: "manifests",
	}
	// Ensure this test covers all defined AllowedManifestFields values
	allFields := AllowedManifestFields(0)
	for k := range fields {
		allFields |= k
	}
	assert.Equal(t, allAllowedFields, allFields)

	// Every single field is allowed by its bit, and rejected by any other bit
	for bit, fieldName := range fields {
		json := []byte(fmt.Sprintf(`{"%s":[]}`, fieldName))
		err := ValidateUnambiguousManifestFormat(json, mt, bit)
		assert.NoError(t, err, fieldName)
		err = ValidateUnambiguousManifestFormat(json, mt, allAllowedFields^bit)
		assert.Error(t, err, fieldName)
	}
}

// Test that parser() rejects all of the provided manifest fixtures.
// Intended to help test manifest parsers' detection of schema mismatches.
func testManifestFixturesAreRejected(t *testing.T, parser func([]byte) error, fixtures []string) {
	for _, fixture := range fixtures {
		manifest, err := os.ReadFile(filepath.Join("testdata", fixture))
		require.NoError(t, err, fixture)
		err = parser(manifest)
		assert.Error(t, err, fixture)
	}
}

// Test that parser() rejects validManifest with an added top-level field with any of the provided field names.
// Intended to help test callers of validateUnambiguousManifestFormat.
func testValidManifestWithExtraFieldsIsRejected(t *testing.T, parser func([]byte) error,
	validManifest []byte, fields []string,
) {
	for _, field := range fields {
		// end (the final '}') is not always at len(validManifest)-1 because the manifest can end with
		// white space.
		end := bytes.LastIndexByte(validManifest, '}')
		require.NotEqual(t, end, -1)
		updatedManifest := []byte(string(validManifest[:end]) +
			fmt.Sprintf(`,"%s":[]}`, field))
		err := parser(updatedManifest)
		// Make sure it is the error from validateUnambiguousManifestFormat, not something that
		// went wrong with creating updatedManifest.
		assert.ErrorContains(t, err, "rejecting ambiguous manifest", field)
	}
}

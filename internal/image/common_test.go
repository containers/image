package image

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertJSONEqualsFixture tests that jsonBytes is structurally equal to fixture,
// possibly ignoring ignoreFields
func assertJSONEqualsFixture(t *testing.T, jsonBytes []byte, fixture string, ignoreFields ...string) {
	var contents map[string]any
	err := json.Unmarshal(jsonBytes, &contents)
	require.NoError(t, err)

	fixtureBytes, err := os.ReadFile(filepath.Join("fixtures", fixture))
	require.NoError(t, err)
	var fixtureContents map[string]any

	err = json.Unmarshal(fixtureBytes, &fixtureContents)
	require.NoError(t, err)
	for _, f := range ignoreFields {
		delete(contents, f)
		delete(fixtureContents, f)
	}
	assert.Equal(t, fixtureContents, contents)
}

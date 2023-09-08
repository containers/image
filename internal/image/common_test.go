package image

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/containers/image/v5/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"
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

// layerInfosWithCryptoOperation returns a copy of input where CryptoOperation is set to op
func layerInfosWithCryptoOperation(input []types.BlobInfo, op types.LayerCrypto) []types.BlobInfo {
	res := slices.Clone(input)
	for i := range res {
		res[i].CryptoOperation = op
	}
	return res
}

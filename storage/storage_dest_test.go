//go:build !containers_image_storage_stub

package storage

import (
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLayerID(t *testing.T) {
	blobDigest, err := digest.Parse("sha256:0000000000000000000000000000000000000000000000000000000000000000")
	require.NoError(t, err)

	for _, c := range []struct {
		parentID        string
		identifiedByTOC bool
		diffID          string
		tocDigest       string
		expected        string
	}{
		{
			parentID:        "",
			identifiedByTOC: false,
			diffID:          "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			tocDigest:       "",
			expected:        "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
		{
			parentID:        "",
			identifiedByTOC: false,
			diffID:          "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			expected:        "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			tocDigest:       "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
		{
			parentID:        "",
			identifiedByTOC: true,
			diffID:          "",
			tocDigest:       "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			expected:        "07f60ddaf18a3d1fa18a71bf40f0b9889b473e26555d6fffdfbd72ba6a59469e",
		},
		{
			parentID:        "",
			identifiedByTOC: true,
			diffID:          "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			tocDigest:       "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			expected:        "07f60ddaf18a3d1fa18a71bf40f0b9889b473e26555d6fffdfbd72ba6a59469e",
		},
		{
			parentID:        "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			identifiedByTOC: false,
			diffID:          "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			tocDigest:       "",
			expected:        "76f79efda453922cda1cecb6ec9e7cf9d86ea968c6dd199d4030dd00078a1686",
		},
		{
			parentID:        "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			identifiedByTOC: false,
			diffID:          "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			tocDigest:       "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
			expected:        "76f79efda453922cda1cecb6ec9e7cf9d86ea968c6dd199d4030dd00078a1686",
		},
		{
			parentID:        "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			identifiedByTOC: true,
			diffID:          "",
			tocDigest:       "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
			expected:        "468becc3d25ee862f81fd728d229a2b2487cfc9b3e6cf3a4d0af8c3fdde0e7a9",
		},
		{
			parentID:        "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			identifiedByTOC: true,
			diffID:          "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			tocDigest:       "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
			expected:        "468becc3d25ee862f81fd728d229a2b2487cfc9b3e6cf3a4d0af8c3fdde0e7a9",
		},
	} {
		var diffID, tocDigest digest.Digest
		if c.diffID != "" {
			diffID, err = digest.Parse(c.diffID)
			require.NoError(t, err)
		}
		if c.tocDigest != "" {
			tocDigest, err = digest.Parse(c.tocDigest)
			require.NoError(t, err)
		}

		res := layerID(c.parentID, trustedLayerIdentityData{
			layerIdentifiedByTOC: c.identifiedByTOC,
			diffID:               diffID,
			tocDigest:            tocDigest,
			blobDigest:           "",
		})
		assert.Equal(t, c.expected, res)
		// blobDigest does not affect the layer ID
		res = layerID(c.parentID, trustedLayerIdentityData{
			layerIdentifiedByTOC: c.identifiedByTOC,
			diffID:               diffID,
			tocDigest:            tocDigest,
			blobDigest:           blobDigest,
		})
		assert.Equal(t, c.expected, res)
	}
}

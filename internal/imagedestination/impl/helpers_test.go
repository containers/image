package impl

import (
	"testing"

	"github.com/containers/image/v5/internal/private"
	"github.com/containers/image/v5/pkg/compression"
	compressionTypes "github.com/containers/image/v5/pkg/compression/types"
	"github.com/stretchr/testify/assert"
)

func TestBlobMatchesRequiredCompression(t *testing.T) {
	var opts private.TryReusingBlobOptions
	cases := []struct {
		requiredCompression  *compressionTypes.Algorithm
		candidateCompression *compressionTypes.Algorithm
		result               bool
	}{
		{&compression.Zstd, &compression.Zstd, true},
		{&compression.Gzip, &compression.Zstd, false},
		{&compression.Zstd, nil, false},
		{nil, &compression.Zstd, true},
	}

	for _, c := range cases {
		opts = private.TryReusingBlobOptions{RequiredCompression: c.requiredCompression}
		assert.Equal(t, BlobMatchesRequiredCompression(opts, c.candidateCompression), c.result)
	}
}

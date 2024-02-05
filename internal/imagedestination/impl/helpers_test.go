package impl

import (
	"testing"

	"github.com/containers/image/v5/internal/private"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/pkg/compression"
	compressionTypes "github.com/containers/image/v5/pkg/compression/types"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
)

func TestCandidateMatchesTryReusingBlobOptions(t *testing.T) {
	cases := []struct {
		requiredCompression     *compressionTypes.Algorithm
		possibleManifestFormats []string
		candidateCompression    *compressionTypes.Algorithm
		result                  bool
	}{
		// RequiredCompression restrictions
		{&compression.Zstd, nil, &compression.Zstd, true},
		{&compression.Gzip, nil, &compression.Zstd, false},
		{&compression.Zstd, nil, nil, false},
		{nil, nil, &compression.Zstd, true},
		// PossibleManifestFormats restrictions
		{nil, []string{imgspecv1.MediaTypeImageManifest}, &compression.Zstd, true},
		{nil, []string{manifest.DockerV2Schema2MediaType}, &compression.Zstd, false},
		{nil, []string{manifest.DockerV2Schema2MediaType, manifest.DockerV2Schema1SignedMediaType, imgspecv1.MediaTypeImageManifest}, &compression.Zstd, true},
		{nil, nil, &compression.Zstd, true},
		{nil, []string{imgspecv1.MediaTypeImageManifest}, &compression.Gzip, true},
		{nil, []string{manifest.DockerV2Schema2MediaType}, &compression.Gzip, true},
		{nil, []string{manifest.DockerV2Schema2MediaType, manifest.DockerV2Schema1SignedMediaType, imgspecv1.MediaTypeImageManifest}, &compression.Gzip, true},
		{nil, nil, &compression.Gzip, true},
		// Some possible combinations (always 1 constraint not matching)
		{&compression.Zstd, []string{manifest.DockerV2Schema2MediaType}, &compression.Zstd, false},
		{&compression.Gzip, []string{manifest.DockerV2Schema2MediaType, manifest.DockerV2Schema1SignedMediaType, imgspecv1.MediaTypeImageManifest}, &compression.Zstd, false},
	}

	for _, c := range cases {
		opts := private.TryReusingBlobOptions{
			RequiredCompression:     c.requiredCompression,
			PossibleManifestFormats: c.possibleManifestFormats,
		}
		assert.Equal(t, c.result, CandidateMatchesTryReusingBlobOptions(opts, c.candidateCompression))
	}
}

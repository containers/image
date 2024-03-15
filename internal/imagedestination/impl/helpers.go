package impl

import (
	"github.com/containers/image/v5/internal/manifest"
	"github.com/containers/image/v5/internal/private"
	compression "github.com/containers/image/v5/pkg/compression/types"
)

// CandidateMatchesTryReusingBlobOptions validates if compression is required by the caller while selecting a blob, if it is required
// then function performs a match against the compression requested by the caller and compression of existing blob
// (which can be nil to represent uncompressed or unknown)
func CandidateMatchesTryReusingBlobOptions(options private.TryReusingBlobOptions, candidateCompression *compression.Algorithm) bool {
	return manifest.CandidateCompressionMatchesReuseConditions(manifest.ReuseConditions{
		PossibleManifestFormats: options.PossibleManifestFormats,
		RequiredCompression:     options.RequiredCompression,
	}, candidateCompression)
}

func OriginalCandidateMatchesTryReusingBlobOptions(opts private.TryReusingBlobOptions) bool {
	return CandidateMatchesTryReusingBlobOptions(opts, opts.OriginalCompression)
}

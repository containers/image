package prioritize

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/containers/image/v5/internal/blobinfocache"
	"github.com/containers/image/v5/pkg/compression"
	compressiontypes "github.com/containers/image/v5/pkg/compression/types"
	"github.com/containers/image/v5/types"
	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	digestUncompressed      = digest.Digest("sha256:2222222222222222222222222222222222222222222222222222222222222222")
	digestCompressedA       = digest.Digest("sha256:3333333333333333333333333333333333333333333333333333333333333333")
	digestCompressedB       = digest.Digest("sha256:4444444444444444444444444444444444444444444444444444444444444444")
	digestCompressedPrimary = digest.Digest("sha256:6666666666666666666666666666666666666666666666666666666666666666")
)

var (
	// inputReplacementCandidates contains a non-trivial candidateSortState shared among several tests below.
	inputReplacementCandidates = []CandidateWithTime{
		{blobinfocache.BICReplacementCandidate2{Digest: digestCompressedA, Location: types.BICLocationReference{Opaque: "A1"}, CompressionOperation: types.Compress, CompressionAlgorithm: &compression.Xz}, time.Unix(1, 0)},
		{blobinfocache.BICReplacementCandidate2{Digest: digestUncompressed, Location: types.BICLocationReference{Opaque: "U2"}, CompressionOperation: types.Compress, CompressionAlgorithm: &compression.Gzip}, time.Unix(1, 1)},
		{blobinfocache.BICReplacementCandidate2{Digest: digestCompressedA, Location: types.BICLocationReference{Opaque: "A2"}, CompressionOperation: types.Decompress, CompressionAlgorithm: nil}, time.Unix(1, 1)},
		{blobinfocache.BICReplacementCandidate2{Digest: digestCompressedPrimary, Location: types.BICLocationReference{Opaque: "P1"}}, time.Unix(1, 0)},
		{blobinfocache.BICReplacementCandidate2{Digest: digestCompressedB, Location: types.BICLocationReference{Opaque: "B1"}, CompressionOperation: types.Compress, CompressionAlgorithm: &compression.Bzip2}, time.Unix(1, 1)},
		{blobinfocache.BICReplacementCandidate2{Digest: digestCompressedPrimary, Location: types.BICLocationReference{Opaque: "P2"}, CompressionOperation: types.Compress, CompressionAlgorithm: &compression.Gzip}, time.Unix(1, 1)},
		{blobinfocache.BICReplacementCandidate2{Digest: digestCompressedB, Location: types.BICLocationReference{Opaque: "B2"}, CompressionOperation: types.Decompress, CompressionAlgorithm: nil}, time.Unix(2, 0)},
		{blobinfocache.BICReplacementCandidate2{Digest: digestUncompressed, Location: types.BICLocationReference{Opaque: "U1"}}, time.Unix(1, 0)},
		{blobinfocache.BICReplacementCandidate2{Digest: digestUncompressed, UnknownLocation: true, Location: types.BICLocationReference{Opaque: ""}}, time.Time{}},
		{blobinfocache.BICReplacementCandidate2{Digest: digestCompressedA, UnknownLocation: true, Location: types.BICLocationReference{Opaque: ""}}, time.Time{}},
		{blobinfocache.BICReplacementCandidate2{Digest: digestCompressedB, UnknownLocation: true, Location: types.BICLocationReference{Opaque: ""}}, time.Time{}},
		{blobinfocache.BICReplacementCandidate2{Digest: digestCompressedPrimary, UnknownLocation: true, Location: types.BICLocationReference{Opaque: ""}}, time.Time{}},
	}
	// expectedReplacementCandidates is the fully-sorted, unlimited, result of prioritizing inputReplacementCandidates.
	expectedReplacementCandidates = []blobinfocache.BICReplacementCandidate2{
		{Digest: digestCompressedPrimary, Location: types.BICLocationReference{Opaque: "P2"}, CompressionOperation: types.Compress, CompressionAlgorithm: &compression.Gzip},
		{Digest: digestCompressedPrimary, Location: types.BICLocationReference{Opaque: "P1"}},
		{Digest: digestCompressedB, Location: types.BICLocationReference{Opaque: "B2"}, CompressionOperation: types.Decompress, CompressionAlgorithm: nil},
		{Digest: digestCompressedA, Location: types.BICLocationReference{Opaque: "A2"}, CompressionOperation: types.Decompress, CompressionAlgorithm: nil},
		{Digest: digestCompressedB, Location: types.BICLocationReference{Opaque: "B1"}, CompressionOperation: types.Compress, CompressionAlgorithm: &compression.Bzip2},
		{Digest: digestCompressedA, Location: types.BICLocationReference{Opaque: "A1"}, CompressionOperation: types.Compress, CompressionAlgorithm: &compression.Xz},
		{Digest: digestUncompressed, Location: types.BICLocationReference{Opaque: "U2"}, CompressionOperation: types.Compress, CompressionAlgorithm: &compression.Gzip},
		{Digest: digestUncompressed, Location: types.BICLocationReference{Opaque: "U1"}},
		{Digest: digestCompressedPrimary, UnknownLocation: true, Location: types.BICLocationReference{Opaque: ""}},
		{Digest: digestCompressedA, UnknownLocation: true, Location: types.BICLocationReference{Opaque: ""}},
		{Digest: digestCompressedB, UnknownLocation: true, Location: types.BICLocationReference{Opaque: ""}},
		{Digest: digestUncompressed, UnknownLocation: true, Location: types.BICLocationReference{Opaque: ""}},
	}
)

func TestCandidateTemplateWithCompression(t *testing.T) {
	chunkedAnnotations := map[string]string{"a": "b"}
	uncompressedData := blobinfocache.DigestCompressorData{
		BaseVariantCompressor:      blobinfocache.Uncompressed,
		SpecificVariantCompressor:  blobinfocache.UnknownCompression,
		SpecificVariantAnnotations: nil,
	}
	gzipData := blobinfocache.DigestCompressorData{
		BaseVariantCompressor:      compressiontypes.GzipAlgorithmName,
		SpecificVariantCompressor:  blobinfocache.UnknownCompression,
		SpecificVariantAnnotations: nil,
	}
	zstdData := blobinfocache.DigestCompressorData{
		BaseVariantCompressor:      compressiontypes.ZstdAlgorithmName,
		SpecificVariantCompressor:  blobinfocache.UnknownCompression,
		SpecificVariantAnnotations: nil,
	}
	zstdChunkedData := blobinfocache.DigestCompressorData{
		BaseVariantCompressor:      compressiontypes.ZstdAlgorithmName,
		SpecificVariantCompressor:  compressiontypes.ZstdChunkedAlgorithmName,
		SpecificVariantAnnotations: chunkedAnnotations,
	}

	for _, c := range []struct {
		name                string
		requiredCompression *compressiontypes.Algorithm
		data                blobinfocache.DigestCompressorData
		v2Matches           bool
		// if v2Matches:
		v2Op          types.LayerCompression
		v2Algo        string
		v2Annotations map[string]string
	}{
		{
			name:                "unknown",
			requiredCompression: nil,
			data: blobinfocache.DigestCompressorData{
				BaseVariantCompressor:      blobinfocache.UnknownCompression,
				SpecificVariantCompressor:  blobinfocache.UnknownCompression,
				SpecificVariantAnnotations: nil,
			},
			v2Matches: false,
		},
		{
			name:                "uncompressed",
			requiredCompression: nil,
			data:                uncompressedData,
			v2Matches:           true,
			v2Op:                types.Decompress,
			v2Algo:              "",
			v2Annotations:       nil,
		},
		{
			name:                "uncompressed, want gzip",
			requiredCompression: &compression.Gzip,
			data:                uncompressedData,
			v2Matches:           false,
		},
		{
			name:                "gzip",
			requiredCompression: nil,
			data:                gzipData,
			v2Matches:           true,
			v2Op:                types.Compress,
			v2Algo:              compressiontypes.GzipAlgorithmName,
			v2Annotations:       nil,
		},
		{
			name:                "gzip, want zstd",
			requiredCompression: &compression.Zstd,
			data:                gzipData,
			v2Matches:           false,
		},
		{
			name:                "unknown base",
			requiredCompression: nil,
			data: blobinfocache.DigestCompressorData{
				BaseVariantCompressor:      "this value is unknown",
				SpecificVariantCompressor:  blobinfocache.UnknownCompression,
				SpecificVariantAnnotations: nil,
			},
			v2Matches: false,
		},
		{
			name:                "zstd",
			requiredCompression: nil,
			data:                zstdData,
			v2Matches:           true,
			v2Op:                types.Compress,
			v2Algo:              compressiontypes.ZstdAlgorithmName,
			v2Annotations:       nil,
		},
		{
			name:                "zstd, want gzip",
			requiredCompression: &compression.Gzip,
			data:                zstdData,
			v2Matches:           false,
		},
		{
			name:                "zstd, want zstd",
			requiredCompression: &compression.Zstd,
			data:                zstdData,
			v2Matches:           true,
			v2Op:                types.Compress,
			v2Algo:              compressiontypes.ZstdAlgorithmName,
			v2Annotations:       nil,
		},
		{
			name:                "zstd, want zstd:chunked",
			requiredCompression: &compression.ZstdChunked,
			data:                zstdData,
			v2Matches:           false,
		},
		{
			name:                "zstd:chunked",
			requiredCompression: nil,
			data:                zstdChunkedData,
			v2Matches:           true,
			v2Op:                types.Compress,
			v2Algo:              compressiontypes.ZstdChunkedAlgorithmName,
			v2Annotations:       chunkedAnnotations,
		},
		{
			name:                "zstd:chunked, want gzip",
			requiredCompression: &compression.Gzip,
			data:                zstdChunkedData,
			v2Matches:           false,
		},
		{
			name:                "zstd:chunked, want zstd", // Note that we return the full chunked data in this case.
			requiredCompression: &compression.Zstd,
			data:                zstdChunkedData,
			v2Matches:           true,
			v2Op:                types.Compress,
			v2Algo:              compressiontypes.ZstdChunkedAlgorithmName,
			v2Annotations:       chunkedAnnotations,
		},
		{
			name:                "zstd:chunked, want zstd:chunked",
			requiredCompression: &compression.ZstdChunked,
			data:                zstdChunkedData,
			v2Matches:           true,
			v2Op:                types.Compress,
			v2Algo:              compressiontypes.ZstdChunkedAlgorithmName,
			v2Annotations:       chunkedAnnotations,
		},
		{
			name:                "zstd:unknown",
			requiredCompression: nil,
			data: blobinfocache.DigestCompressorData{
				BaseVariantCompressor:      compressiontypes.ZstdAlgorithmName,
				SpecificVariantCompressor:  "this value is unknown",
				SpecificVariantAnnotations: chunkedAnnotations,
			},
			v2Matches:     true,
			v2Op:          types.Compress,
			v2Algo:        compressiontypes.ZstdAlgorithmName,
			v2Annotations: nil,
		},
	} {
		res := CandidateTemplateWithCompression(nil, digestCompressedPrimary, c.data)
		assert.Equal(t, &CandidateTemplate{
			digest:                 digestCompressedPrimary,
			compressionOperation:   types.PreserveOriginal,
			compressionAlgorithm:   nil,
			compressionAnnotations: nil,
		}, res, c.name)

		// These tests only use RequiredCompression in CandidateLocations2Options for clarity;
		// CandidateCompressionMatchesReuseConditions should have its own tests of handling the full set of options.
		res = CandidateTemplateWithCompression(&blobinfocache.CandidateLocations2Options{
			RequiredCompression: c.requiredCompression,
		}, digestCompressedPrimary, c.data)
		if !c.v2Matches {
			assert.Nil(t, res, c.name)
		} else {
			require.NotNil(t, res, c.name)
			assert.Equal(t, digestCompressedPrimary, res.digest, c.name)
			assert.Equal(t, c.v2Op, res.compressionOperation, c.name)
			if c.v2Algo == "" {
				assert.Nil(t, res.compressionAlgorithm, c.name)
			} else {
				require.NotNil(t, res.compressionAlgorithm, c.name)
				assert.Equal(t, c.v2Algo, res.compressionAlgorithm.Name())
			}
			assert.Equal(t, c.v2Annotations, res.compressionAnnotations, c.name)
		}
	}
}

func TestCandidateWithLocation(t *testing.T) {
	template := CandidateTemplateWithCompression(&blobinfocache.CandidateLocations2Options{}, digestCompressedPrimary, blobinfocache.DigestCompressorData{
		BaseVariantCompressor:      compressiontypes.ZstdAlgorithmName,
		SpecificVariantCompressor:  compressiontypes.ZstdChunkedAlgorithmName,
		SpecificVariantAnnotations: map[string]string{"a": "b"},
	})
	require.NotNil(t, template)
	loc := types.BICLocationReference{Opaque: "opaque"}
	time := time.Now()
	res := template.CandidateWithLocation(loc, time)
	assert.Equal(t, digestCompressedPrimary, res.candidate.Digest)
	assert.Equal(t, types.Compress, res.candidate.CompressionOperation)
	assert.Equal(t, compressiontypes.ZstdChunkedAlgorithmName, res.candidate.CompressionAlgorithm.Name())
	assert.Equal(t, map[string]string{"a": "b"}, res.candidate.CompressionAnnotations)
	assert.Equal(t, false, res.candidate.UnknownLocation)
	assert.Equal(t, loc, res.candidate.Location)
	assert.Equal(t, time, res.lastSeen)
}

func TestCandidateWithUnknownLocation(t *testing.T) {
	template := CandidateTemplateWithCompression(&blobinfocache.CandidateLocations2Options{}, digestCompressedPrimary, blobinfocache.DigestCompressorData{
		BaseVariantCompressor:      compressiontypes.ZstdAlgorithmName,
		SpecificVariantCompressor:  compressiontypes.ZstdChunkedAlgorithmName,
		SpecificVariantAnnotations: map[string]string{"a": "b"},
	})
	require.NotNil(t, template)
	res := template.CandidateWithUnknownLocation()
	assert.Equal(t, digestCompressedPrimary, res.candidate.Digest)
	assert.Equal(t, types.Compress, res.candidate.CompressionOperation)
	assert.Equal(t, compressiontypes.ZstdChunkedAlgorithmName, res.candidate.CompressionAlgorithm.Name())
	assert.Equal(t, map[string]string{"a": "b"}, res.candidate.CompressionAnnotations)
	assert.Equal(t, true, res.candidate.UnknownLocation)
}

func TestCandidateSortStateLess(t *testing.T) {
	type p struct {
		d digest.Digest
		t int64
	}

	// Primary criteria: Also ensure that time does not matter
	for _, c := range []struct {
		name   string
		res    int
		d0, d1 digest.Digest
	}{
		{"primary < any", -1, digestCompressedPrimary, digestCompressedA},
		{"any < uncompressed", -1, digestCompressedA, digestUncompressed},
		{"primary < uncompressed", -1, digestCompressedPrimary, digestUncompressed},
	} {
		for _, tms := range [][2]int64{{1, 2}, {2, 1}, {1, 1}} {
			caseName := fmt.Sprintf("%s %v", c.name, tms)
			c0 := CandidateWithTime{blobinfocache.BICReplacementCandidate2{Digest: c.d0, Location: types.BICLocationReference{Opaque: "L0"}, CompressionOperation: types.Compress, CompressionAlgorithm: &compression.Gzip}, time.Unix(tms[0], 0)}
			c1 := CandidateWithTime{blobinfocache.BICReplacementCandidate2{Digest: c.d1, Location: types.BICLocationReference{Opaque: "L1"}, CompressionOperation: types.Compress, CompressionAlgorithm: &compression.Zstd}, time.Unix(tms[1], 0)}
			css := candidateSortState{
				primaryDigest:      digestCompressedPrimary,
				uncompressedDigest: digestUncompressed,
			}
			assert.Equal(t, c.res, css.compare(c0, c1), caseName)
			assert.Equal(t, -c.res, css.compare(c1, c0), caseName)

			if c.d0 != digestUncompressed && c.d1 != digestUncompressed {
				css.uncompressedDigest = ""
				assert.Equal(t, c.res, css.compare(c0, c1), caseName)
				assert.Equal(t, -c.res, css.compare(c1, c0), caseName)

				css.uncompressedDigest = css.primaryDigest
				assert.Equal(t, c.res, css.compare(c0, c1), caseName)
				assert.Equal(t, -c.res, css.compare(c1, c0), caseName)
			}
		}
	}

	// Ordering within the three primary groups
	for _, c := range []struct {
		name   string
		res    int
		p0, p1 p
	}{
		{"primary: t=2 < t=1", -1, p{digestCompressedPrimary, 2}, p{digestCompressedPrimary, 1}},
		{"primary: t=1 == t=1", 0, p{digestCompressedPrimary, 1}, p{digestCompressedPrimary, 1}},
		{"uncompressed: t=2 < t=1", -1, p{digestUncompressed, 2}, p{digestUncompressed, 1}},
		{"uncompressed: t=1 == t=1", 0, p{digestUncompressed, 1}, p{digestUncompressed, 1}},
		{"any: t=2 < t=1, [d=A vs. d=B lower-priority]", -1, p{digestCompressedA, 2}, p{digestCompressedB, 1}},
		{"any: t=2 < t=1, [d=B vs. d=A lower-priority]", -1, p{digestCompressedB, 2}, p{digestCompressedA, 1}},
		{"any: t=2 < t=1, [d=A vs. d=A lower-priority]", -1, p{digestCompressedA, 2}, p{digestCompressedA, 1}},
		{"any: t=1 == t=1, d=A < d=B", -1, p{digestCompressedA, 1}, p{digestCompressedB, 1}},
		{"any: t=1 == t=1, d=A == d=A", 0, p{digestCompressedA, 1}, p{digestCompressedA, 1}},
	} {
		c0 := CandidateWithTime{blobinfocache.BICReplacementCandidate2{Digest: c.p0.d, Location: types.BICLocationReference{Opaque: "L0"}, CompressionOperation: types.Compress, CompressionAlgorithm: &compression.Gzip}, time.Unix(c.p0.t, 0)}
		c1 := CandidateWithTime{blobinfocache.BICReplacementCandidate2{Digest: c.p1.d, Location: types.BICLocationReference{Opaque: "L1"}, CompressionOperation: types.Compress, CompressionAlgorithm: &compression.Zstd}, time.Unix(c.p1.t, 0)}
		css := candidateSortState{
			primaryDigest:      digestCompressedPrimary,
			uncompressedDigest: digestUncompressed,
		}
		assert.Equal(t, c.res, css.compare(c0, c1), c.name)
		assert.Equal(t, -c.res, css.compare(c1, c0), c.name)

		if c.p0.d != digestUncompressed && c.p1.d != digestUncompressed {
			css.uncompressedDigest = ""
			assert.Equal(t, c.res, css.compare(c0, c1), c.name)
			assert.Equal(t, -c.res, css.compare(c1, c0), c.name)

			css.uncompressedDigest = css.primaryDigest
			assert.Equal(t, c.res, css.compare(c0, c1), c.name)
			assert.Equal(t, -c.res, css.compare(c1, c0), c.name)
		}
	}
}

func TestDestructivelyPrioritizeReplacementCandidatesWithMax(t *testing.T) {
	totalUnknownLocationCandidates := 4
	for _, totalLimit := range []int{0, 1, replacementAttempts, 100, replacementUnknownLocationAttempts} {
		for _, noLocationLimit := range []int{0, 1, replacementAttempts, 100, replacementUnknownLocationAttempts} {
			totalKnownLocationCandidates := len(expectedReplacementCandidates) - totalUnknownLocationCandidates
			allowedUnknown := min(noLocationLimit, totalUnknownLocationCandidates)
			expectedLen := min(totalKnownLocationCandidates+allowedUnknown, totalLimit)
			res := destructivelyPrioritizeReplacementCandidatesWithMax(slices.Clone(inputReplacementCandidates), digestCompressedPrimary, digestUncompressed, totalLimit, noLocationLimit)
			assert.Equal(t, expectedReplacementCandidates[:expectedLen], res)
		}
	}
}

func TestDestructivelyPrioritizeReplacementCandidates(t *testing.T) {
	// Just a smoke test; we mostly rely on test coverage in TestCandidateSortStateLess
	res := DestructivelyPrioritizeReplacementCandidates(slices.Clone(inputReplacementCandidates), digestCompressedPrimary, digestUncompressed)
	assert.Equal(t, expectedReplacementCandidates[:replacementAttempts], res)
}

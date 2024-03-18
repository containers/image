package prioritize

import (
	"fmt"
	"testing"
	"time"

	"github.com/containers/image/v5/internal/blobinfocache"
	"github.com/containers/image/v5/pkg/compression"
	"github.com/containers/image/v5/types"
	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"golang.org/x/exp/slices"
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

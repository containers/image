// Package test provides generic BlobInfoCache test helpers.
package test

import (
	"testing"

	"github.com/containers/image/v5/internal/blobinfocache"
	"github.com/containers/image/v5/internal/testing/mocks"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/pkg/compression"
	compressiontypes "github.com/containers/image/v5/pkg/compression/types"
	"github.com/containers/image/v5/types"
	digest "github.com/opencontainers/go-digest"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	digestUnknown             = digest.Digest("sha256:1111111111111111111111111111111111111111111111111111111111111111")
	digestUncompressed        = digest.Digest("sha256:2222222222222222222222222222222222222222222222222222222222222222")
	digestCompressedA         = digest.Digest("sha256:3333333333333333333333333333333333333333333333333333333333333333")
	digestCompressedB         = digest.Digest("sha256:4444444444444444444444444444444444444444444444444444444444444444")
	digestCompressedUnrelated = digest.Digest("sha256:5555555555555555555555555555555555555555555555555555555555555555")
	compressorNameU           = blobinfocache.Uncompressed
	compressorNameA           = compressiontypes.GzipAlgorithmName
	compressorNameB           = compressiontypes.ZstdAlgorithmName
	compressorNameCU          = compressiontypes.XzAlgorithmName

	digestUnknownLocation       = digest.Digest("sha256:7777777777777777777777777777777777777777777777777777777777777777")
	digestFilteringUncompressed = digest.Digest("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	digestGzip                  = digest.Digest("sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	digestZstd                  = digest.Digest("sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc")
	digestZstdChunked           = digest.Digest("sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd")
)

// GenericCache runs an implementation-independent set of tests, given a
// newTestCache, which can be called repeatedly and always returns a fresh cache instance
func GenericCache(t *testing.T, newTestCache func(t *testing.T) blobinfocache.BlobInfoCache2) {
	subs := []struct {
		name string
		fn   func(t *testing.T, cache blobinfocache.BlobInfoCache2)
	}{
		{"UncompressedDigest", testGenericUncompressedDigest},
		{"RecordDigestUncompressedPair", testGenericRecordDigestUncompressedPair},
		{"UncompressedDigestForTOC", testGenericUncompressedDigestForTOC},
		{"RecordTOCUncompressedPair", testGenericRecordTOCUncompressedPair},
		{"RecordKnownLocations", testGenericRecordKnownLocations},
		{"CandidateLocations", testGenericCandidateLocations},
		{"CandidateLocations2", testGenericCandidateLocations2},
	}

	// Without Open()/Close()
	for _, s := range subs {
		t.Run("no Open: "+s.name, func(t *testing.T) {
			cache := newTestCache(t)
			s.fn(t, cache)
		})
	}

	// With Open()/Close()
	for _, s := range subs {
		t.Run("with Open: "+s.name, func(t *testing.T) {
			cache := newTestCache(t)
			cache.Open()
			defer cache.Close()
			s.fn(t, cache)
		})
	}
}

func testGenericUncompressedDigest(t *testing.T, cache blobinfocache.BlobInfoCache2) {
	// Nothing is known.
	assert.Equal(t, digest.Digest(""), cache.UncompressedDigest(digestUnknown))

	cache.RecordDigestUncompressedPair(digestCompressedA, digestUncompressed)
	cache.RecordDigestUncompressedPair(digestCompressedB, digestUncompressed)
	// Known compressed→uncompressed mapping
	assert.Equal(t, digestUncompressed, cache.UncompressedDigest(digestCompressedA))
	assert.Equal(t, digestUncompressed, cache.UncompressedDigest(digestCompressedB))
	// This implicitly marks digestUncompressed as uncompressed.
	assert.Equal(t, digestUncompressed, cache.UncompressedDigest(digestUncompressed))

	// Known uncompressed→self mapping
	cache.RecordDigestUncompressedPair(digestCompressedUnrelated, digestCompressedUnrelated)
	assert.Equal(t, digestCompressedUnrelated, cache.UncompressedDigest(digestCompressedUnrelated))
}

func testGenericRecordDigestUncompressedPair(t *testing.T, cache blobinfocache.BlobInfoCache2) {
	for i := 0; i < 2; i++ { // Record the same data twice to ensure redundant writes don’t break things.
		// Known compressed→uncompressed mapping
		cache.RecordDigestUncompressedPair(digestCompressedA, digestUncompressed)
		assert.Equal(t, digestUncompressed, cache.UncompressedDigest(digestCompressedA))
		// Two mappings to the same uncompressed digest
		cache.RecordDigestUncompressedPair(digestCompressedB, digestUncompressed)
		assert.Equal(t, digestUncompressed, cache.UncompressedDigest(digestCompressedB))

		// Mapping an uncompressed digest to self
		cache.RecordDigestUncompressedPair(digestUncompressed, digestUncompressed)
		assert.Equal(t, digestUncompressed, cache.UncompressedDigest(digestUncompressed))
	}
}

func testGenericUncompressedDigestForTOC(t *testing.T, cache blobinfocache.BlobInfoCache2) {
	// Nothing is known.
	assert.Equal(t, digest.Digest(""), cache.UncompressedDigestForTOC(digestUnknown))

	cache.RecordTOCUncompressedPair(digestCompressedA, digestUncompressed)
	cache.RecordTOCUncompressedPair(digestCompressedB, digestUncompressed)
	// Known TOC→uncompressed mapping
	assert.Equal(t, digestUncompressed, cache.UncompressedDigestForTOC(digestCompressedA))
	assert.Equal(t, digestUncompressed, cache.UncompressedDigestForTOC(digestCompressedB))
}

func testGenericRecordTOCUncompressedPair(t *testing.T, cache blobinfocache.BlobInfoCache2) {
	for i := 0; i < 2; i++ { // Record the same data twice to ensure redundant writes don’t break things.
		// Known TOC→uncompressed mapping
		cache.RecordTOCUncompressedPair(digestCompressedA, digestUncompressed)
		assert.Equal(t, digestUncompressed, cache.UncompressedDigestForTOC(digestCompressedA))
		// Two mappings to the same uncompressed digest
		cache.RecordTOCUncompressedPair(digestCompressedB, digestUncompressed)
		assert.Equal(t, digestUncompressed, cache.UncompressedDigestForTOC(digestCompressedB))
	}
}

func testGenericRecordKnownLocations(t *testing.T, cache blobinfocache.BlobInfoCache2) {
	transport := mocks.NameImageTransport("==BlobInfocache transport mock")
	for i := 0; i < 2; i++ { // Record the same data twice to ensure redundant writes don’t break things.
		for _, scopeName := range []string{"A", "B"} { // Run the test in two different scopes to verify they don't affect each other.
			scope := types.BICTransportScope{Opaque: scopeName}
			for _, digest := range []digest.Digest{digestCompressedA, digestCompressedB} { // Two different digests should not affect each other either.
				lr1 := types.BICLocationReference{Opaque: scopeName + "1"}
				lr2 := types.BICLocationReference{Opaque: scopeName + "2"}
				cache.RecordKnownLocation(transport, scope, digest, lr2)
				cache.RecordKnownLocation(transport, scope, digest, lr1)
				assert.Equal(t, []types.BICReplacementCandidate{
					{Digest: digest, Location: lr1},
					{Digest: digest, Location: lr2},
				}, cache.CandidateLocations(transport, scope, digest, false))
				res := cache.CandidateLocations2(transport, scope, digest, blobinfocache.CandidateLocations2Options{
					CanSubstitute: false,
				})
				assert.Equal(t, []blobinfocache.BICReplacementCandidate2{}, res)
			}
		}
	}
}

// candidate is a shorthand for types.BICReplacementCandidate
type candidate struct {
	d  digest.Digest
	cn string
	ca map[string]string
	lr string
}

func assertCandidatesMatch(t *testing.T, scopeName string, expected []candidate, actual []types.BICReplacementCandidate) {
	e := make([]types.BICReplacementCandidate, len(expected))
	for i, ev := range expected {
		e[i] = types.BICReplacementCandidate{Digest: ev.d, Location: types.BICLocationReference{Opaque: scopeName + ev.lr}}
	}
	assert.Equal(t, e, actual)
}

func assertCandidateMatches2(t *testing.T, expected, actual blobinfocache.BICReplacementCandidate2) {
	// Verify actual[i].CompressionAlgorithm separately; assert.Equal would do a pointer comparison, and fail.
	if expected.CompressionAlgorithm != nil {
		require.NotNil(t, actual.CompressionAlgorithm)
		assert.Equal(t, expected.CompressionAlgorithm.Name(), actual.CompressionAlgorithm.Name())
	} else {
		assert.Nil(t, actual.CompressionAlgorithm)
	}
	c := expected                                        // A shallow copy
	c.CompressionAlgorithm = actual.CompressionAlgorithm // Already verified above

	assert.Equal(t, c, actual)
}

func assertCandidatesMatch2Native(t *testing.T, expected, actual []blobinfocache.BICReplacementCandidate2) {
	assert.Len(t, actual, len(expected))
	for i := range expected {
		assertCandidateMatches2(t, expected[i], actual[i])
	}
}

func assertCandidatesMatch2(t *testing.T, scopeName string, expected []candidate, actual []blobinfocache.BICReplacementCandidate2) {
	e := make([]blobinfocache.BICReplacementCandidate2, len(expected))
	for i, ev := range expected {
		op := types.Decompress
		var algo *compressiontypes.Algorithm = nil
		if ev.cn != blobinfocache.Uncompressed {
			algo_, err := compression.AlgorithmByName(ev.cn)
			require.NoError(t, err)
			op = types.Compress
			algo = &algo_
		}
		e[i] = blobinfocache.BICReplacementCandidate2{
			Digest:                 ev.d,
			CompressionOperation:   op,
			CompressionAlgorithm:   algo,
			CompressionAnnotations: ev.ca,
			UnknownLocation:        false,
			Location:               types.BICLocationReference{Opaque: scopeName + ev.lr},
		}
	}
	assertCandidatesMatch2Native(t, e, actual)
}

func testGenericCandidateLocations(t *testing.T, cache blobinfocache.BlobInfoCache2) {
	transport := mocks.NameImageTransport("==BlobInfocache transport mock")
	cache.RecordDigestUncompressedPair(digestCompressedA, digestUncompressed)
	cache.RecordDigestUncompressedPair(digestCompressedB, digestUncompressed)
	cache.RecordDigestUncompressedPair(digestUncompressed, digestUncompressed)
	digestNameSet := []struct {
		n string
		d digest.Digest
	}{
		{"U", digestUncompressed},
		{"A", digestCompressedA},
		{"B", digestCompressedB},
		{"CU", digestCompressedUnrelated},
	}

	for _, scopeName := range []string{"A", "B"} { // Run the test in two different scopes to verify they don't affect each other.
		scope := types.BICTransportScope{Opaque: scopeName}
		// Nothing is known.
		assert.Equal(t, []types.BICReplacementCandidate{}, cache.CandidateLocations(transport, scope, digestUnknown, false))
		assert.Equal(t, []types.BICReplacementCandidate{}, cache.CandidateLocations(transport, scope, digestUnknown, true))

		// Record "2" entries before "1" entries; then results should sort "1" (more recent) before "2" (older)
		for _, suffix := range []string{"2", "1"} {
			for _, e := range digestNameSet {
				cache.RecordKnownLocation(transport, scope, e.d, types.BICLocationReference{Opaque: scopeName + e.n + suffix})
			}
		}

		// No substitutions allowed:
		for _, e := range digestNameSet {
			assertCandidatesMatch(t, scopeName, []candidate{
				{d: e.d, lr: e.n + "1"}, {d: e.d, lr: e.n + "2"},
			}, cache.CandidateLocations(transport, scope, e.d, false))
		}

		// With substitutions: The original digest is always preferred, then other compressed, then the uncompressed one.
		assertCandidatesMatch(t, scopeName, []candidate{
			{d: digestCompressedA, lr: "A1"}, {d: digestCompressedA, lr: "A2"},
			{d: digestCompressedB, lr: "B1"}, {d: digestCompressedB, lr: "B2"},
			{d: digestUncompressed, lr: "U1"}, // Beyond the replacementAttempts limit: {d: digestUncompressed, lr: "U2"},
		}, cache.CandidateLocations(transport, scope, digestCompressedA, true))

		assertCandidatesMatch(t, scopeName, []candidate{
			{d: digestCompressedB, lr: "B1"}, {d: digestCompressedB, lr: "B2"},
			{d: digestCompressedA, lr: "A1"}, {d: digestCompressedA, lr: "A2"},
			{d: digestUncompressed, lr: "U1"}, // Beyond the replacementAttempts limit: {d: digestUncompressed, lr: "U2"},
		}, cache.CandidateLocations(transport, scope, digestCompressedB, true))

		assertCandidatesMatch(t, scopeName, []candidate{
			{d: digestUncompressed, lr: "U1"}, {d: digestUncompressed, lr: "U2"},
			// "1" entries were added after "2", and A/Bs are sorted in the reverse of digestNameSet order
			{d: digestCompressedB, lr: "B1"},
			{d: digestCompressedA, lr: "A1"},
			{d: digestCompressedB, lr: "B2"},
			// Beyond the replacementAttempts limit: {d: digestCompressedA, lr: "A2"},
		}, cache.CandidateLocations(transport, scope, digestUncompressed, true))

		// Locations are known, but no relationships
		assertCandidatesMatch(t, scopeName, []candidate{
			{d: digestCompressedUnrelated, lr: "CU1"}, {d: digestCompressedUnrelated, lr: "CU2"},
		}, cache.CandidateLocations(transport, scope, digestCompressedUnrelated, true))
	}
}

func testGenericCandidateLocations2(t *testing.T, cache blobinfocache.BlobInfoCache2) {
	transport := mocks.NameImageTransport("==BlobInfocache transport mock")
	cache.RecordDigestUncompressedPair(digestCompressedA, digestUncompressed)
	cache.RecordDigestUncompressedPair(digestCompressedB, digestUncompressed)
	cache.RecordDigestUncompressedPair(digestUncompressed, digestUncompressed)
	digestNameSetPrioritization := []struct { // Used primarily to test prioritization
		n string
		d digest.Digest
		m string
	}{
		{"U", digestUncompressed, compressorNameU},
		{"A", digestCompressedA, compressorNameA},
		{"B", digestCompressedB, compressorNameB},
		{"CU", digestCompressedUnrelated, compressorNameCU},
	}
	chunkedAnnotations := map[string]string{"a": "b"}
	digestNameSetFiltering := []struct { // Used primarily to test filtering in CandidateLocations2Options
		n              string
		d              digest.Digest
		base, specific string
		annotations    map[string]string
	}{
		{"gzip", digestGzip, compressiontypes.GzipAlgorithmName, blobinfocache.UnknownCompression, nil},
		{"zstd", digestZstd, compressiontypes.ZstdAlgorithmName, blobinfocache.UnknownCompression, nil},
		{"zstdChunked", digestZstdChunked, compressiontypes.ZstdAlgorithmName, compressiontypes.ZstdChunkedAlgorithmName, chunkedAnnotations},
	}
	for _, e := range digestNameSetFiltering { // digestFilteringUncompressed exists only to allow the three entries to be considered as candidates
		cache.RecordDigestUncompressedPair(e.d, digestFilteringUncompressed)
	}

	for scopeIndex, scopeName := range []string{"A", "B", "C"} { // Run the test in two different scopes to verify they don't affect each other.
		scope := types.BICTransportScope{Opaque: scopeName}

		// Nothing is known.
		// -----------------
		res := cache.CandidateLocations2(transport, scope, digestUnknown, blobinfocache.CandidateLocations2Options{
			CanSubstitute: false,
		})
		assert.Equal(t, []blobinfocache.BICReplacementCandidate2{}, res)
		res = cache.CandidateLocations2(transport, scope, digestUnknown, blobinfocache.CandidateLocations2Options{
			CanSubstitute: true,
		})
		assert.Equal(t, []blobinfocache.BICReplacementCandidate2{}, res)

		// Results with UnknownLocation
		// ----------------------------
		// If a record exists with compression without Location then
		// then return a record without location and with `UnknownLocation: true`
		cache.RecordDigestCompressorData(digestUnknownLocation, blobinfocache.DigestCompressorData{
			BaseVariantCompressor:      compressiontypes.Bzip2AlgorithmName,
			SpecificVariantCompressor:  blobinfocache.UnknownCompression,
			SpecificVariantAnnotations: nil,
		})
		res = cache.CandidateLocations2(transport, scope, digestUnknownLocation, blobinfocache.CandidateLocations2Options{
			CanSubstitute: true,
		})
		assertCandidatesMatch2Native(t, []blobinfocache.BICReplacementCandidate2{
			{
				Digest:               digestUnknownLocation,
				CompressionOperation: types.Compress,
				CompressionAlgorithm: &compression.Bzip2,
				UnknownLocation:      true,
				Location:             types.BICLocationReference{Opaque: ""},
			}}, res)
		// When another entry with scope and Location is set then it should be returned as it has higher
		// priority.
		cache.RecordKnownLocation(transport, scope, digestUnknownLocation, types.BICLocationReference{Opaque: "somelocation"})
		res = cache.CandidateLocations2(transport, scope, digestUnknownLocation, blobinfocache.CandidateLocations2Options{
			CanSubstitute: true,
		})
		assertCandidatesMatch2Native(t, []blobinfocache.BICReplacementCandidate2{
			{
				Digest:               digestUnknownLocation,
				CompressionOperation: types.Compress,
				CompressionAlgorithm: &compression.Bzip2,
				UnknownLocation:      false,
				Location:             types.BICLocationReference{Opaque: "somelocation"},
			}}, res)

		// Tests of lookups / prioritization when compression is unknown
		// -------------------------------------------------------------

		// Record "2" entries before "1" entries; then results should sort "1" (more recent) before "2" (older)
		for _, suffix := range []string{"2", "1"} {
			for _, e := range digestNameSetPrioritization {
				cache.RecordKnownLocation(transport, scope, e.d, types.BICLocationReference{Opaque: scopeName + e.n + suffix})
			}
		}
		// Clear any "known" compression values, except on the first loop where they've never been set.
		// This probably triggers “Compressor for blob with digest … previously recorded as …, now unknown” warnings here, for test purposes;
		// that shouldn’t happen in real-world usage.
		if scopeIndex != 0 {
			for _, e := range digestNameSetPrioritization {
				cache.RecordDigestCompressorData(e.d, blobinfocache.DigestCompressorData{
					BaseVariantCompressor:      blobinfocache.UnknownCompression,
					SpecificVariantCompressor:  blobinfocache.UnknownCompression,
					SpecificVariantAnnotations: nil,
				})
			}
		}

		// No substitutions allowed:
		for _, e := range digestNameSetPrioritization {
			assertCandidatesMatch(t, scopeName, []candidate{
				{d: e.d, lr: e.n + "1"}, {d: e.d, lr: e.n + "2"},
			}, cache.CandidateLocations(transport, scope, e.d, false))
			// Unknown compression -> no candidates
			res := cache.CandidateLocations2(transport, scope, e.d, blobinfocache.CandidateLocations2Options{
				CanSubstitute: false,
			})
			assertCandidatesMatch2(t, scopeName, []candidate{}, res)
		}

		// With substitutions: The original digest is always preferred, then other compressed, then the uncompressed one.
		assertCandidatesMatch(t, scopeName, []candidate{
			{d: digestCompressedA, lr: "A1"}, {d: digestCompressedA, lr: "A2"},
			{d: digestCompressedB, lr: "B1"}, {d: digestCompressedB, lr: "B2"},
			{d: digestUncompressed, lr: "U1"}, // Beyond the replacementAttempts limit: {d: digestUncompressed, cn: compressorNameCU, lr: "U2"},
		}, cache.CandidateLocations(transport, scope, digestCompressedA, true))
		// Unknown compression -> no candidates
		res = cache.CandidateLocations2(transport, scope, digestCompressedA, blobinfocache.CandidateLocations2Options{
			CanSubstitute: true,
		})
		assertCandidatesMatch2(t, scopeName, []candidate{}, res)

		assertCandidatesMatch(t, scopeName, []candidate{
			{d: digestCompressedB, lr: "B1"}, {d: digestCompressedB, lr: "B2"},
			{d: digestCompressedA, lr: "A1"}, {d: digestCompressedA, lr: "A2"},
			{d: digestUncompressed, lr: "U1"}, // Beyond the replacementAttempts limit: {d: digestUncompressed, lr: "U2"},
		}, cache.CandidateLocations(transport, scope, digestCompressedB, true))
		// Unknown compression -> no candidates
		res = cache.CandidateLocations2(transport, scope, digestCompressedB, blobinfocache.CandidateLocations2Options{
			CanSubstitute: true,
		})
		assertCandidatesMatch2(t, scopeName, []candidate{}, res)

		assertCandidatesMatch(t, scopeName, []candidate{
			{d: digestUncompressed, lr: "U1"}, {d: digestUncompressed, lr: "U2"},
			// "1" entries were added after "2", and A/Bs are sorted in the reverse of digestNameSetPrioritization order
			{d: digestCompressedB, lr: "B1"},
			{d: digestCompressedA, lr: "A1"},
			{d: digestCompressedB, lr: "B2"},
			// Beyond the replacementAttempts limit: {d: digestCompressedA, lr: "A2"},
		}, cache.CandidateLocations(transport, scope, digestUncompressed, true))
		// Unknown compression -> no candidates
		res = cache.CandidateLocations2(transport, scope, digestUncompressed, blobinfocache.CandidateLocations2Options{
			CanSubstitute: true,
		})
		assertCandidatesMatch2(t, scopeName, []candidate{}, res)

		// Locations are known, but no relationships
		assertCandidatesMatch(t, scopeName, []candidate{
			{d: digestCompressedUnrelated, lr: "CU1"}, {d: digestCompressedUnrelated, lr: "CU2"},
		}, cache.CandidateLocations(transport, scope, digestCompressedUnrelated, true))
		// Unknown compression -> no candidates
		res = cache.CandidateLocations2(transport, scope, digestCompressedUnrelated, blobinfocache.CandidateLocations2Options{
			CanSubstitute: true,
		})
		assertCandidatesMatch2(t, scopeName, []candidate{}, res)

		// Tests of lookups / prioritization when compression is known
		// -------------------------------------------------------------

		// Set the "known" compression values
		for _, e := range digestNameSetPrioritization {
			cache.RecordDigestCompressorData(e.d, blobinfocache.DigestCompressorData{
				BaseVariantCompressor:      e.m,
				SpecificVariantCompressor:  blobinfocache.UnknownCompression,
				SpecificVariantAnnotations: nil,
			})
		}

		// No substitutions allowed:
		for _, e := range digestNameSetPrioritization {
			assertCandidatesMatch(t, scopeName, []candidate{
				{d: e.d, lr: e.n + "1"}, {d: e.d, lr: e.n + "2"},
			}, cache.CandidateLocations(transport, scope, e.d, false))
			res := cache.CandidateLocations2(transport, scope, e.d, blobinfocache.CandidateLocations2Options{
				CanSubstitute: false,
			})
			assertCandidatesMatch2(t, scopeName, []candidate{
				{d: e.d, cn: e.m, lr: e.n + "1"}, {d: e.d, cn: e.m, lr: e.n + "2"},
			}, res)
		}

		// With substitutions: The original digest is always preferred, then other compressed, then the uncompressed one.
		assertCandidatesMatch(t, scopeName, []candidate{
			{d: digestCompressedA, lr: "A1"}, {d: digestCompressedA, lr: "A2"},
			{d: digestCompressedB, lr: "B1"}, {d: digestCompressedB, lr: "B2"},
			{d: digestUncompressed, lr: "U1"}, // Beyond the replacementAttempts limit: {d: digestUncompressed, lr: "U2"},
		}, cache.CandidateLocations(transport, scope, digestCompressedA, true))
		res = cache.CandidateLocations2(transport, scope, digestCompressedA, blobinfocache.CandidateLocations2Options{
			CanSubstitute: true,
		})
		assertCandidatesMatch2(t, scopeName, []candidate{
			{d: digestCompressedA, cn: compressorNameA, lr: "A1"}, {d: digestCompressedA, cn: compressorNameA, lr: "A2"},
			{d: digestCompressedB, cn: compressorNameB, lr: "B1"}, {d: digestCompressedB, cn: compressorNameB, lr: "B2"},
			{d: digestUncompressed, cn: compressorNameU, lr: "U1"}, // Beyond the replacementAttempts limit: {d: digestUncompressed, cn: compressorNameCU, lr: "U2"},
		}, res)

		assertCandidatesMatch(t, scopeName, []candidate{
			{d: digestCompressedB, lr: "B1"}, {d: digestCompressedB, lr: "B2"},
			{d: digestCompressedA, lr: "A1"}, {d: digestCompressedA, lr: "A2"},
			{d: digestUncompressed, lr: "U1"}, // Beyond the replacementAttempts limit: {d: digestUncompressed, lr: "U2"},
		}, cache.CandidateLocations(transport, scope, digestCompressedB, true))
		res = cache.CandidateLocations2(transport, scope, digestCompressedB, blobinfocache.CandidateLocations2Options{
			CanSubstitute: true,
		})
		assertCandidatesMatch2(t, scopeName, []candidate{
			{d: digestCompressedB, cn: compressorNameB, lr: "B1"}, {d: digestCompressedB, cn: compressorNameB, lr: "B2"},
			{d: digestCompressedA, cn: compressorNameA, lr: "A1"}, {d: digestCompressedA, cn: compressorNameA, lr: "A2"},
			{d: digestUncompressed, cn: compressorNameU, lr: "U1"}, // Beyond the replacementAttempts limit: {d: digestUncompressed, cn: compressorNameU, lr: "U2"},
		}, res)

		assertCandidatesMatch(t, scopeName, []candidate{
			{d: digestUncompressed, lr: "U1"}, {d: digestUncompressed, lr: "U2"},
			// "1" entries were added after "2", and A/Bs are sorted in the reverse of digestNameSetPrioritization order
			{d: digestCompressedB, lr: "B1"},
			{d: digestCompressedA, lr: "A1"},
			{d: digestCompressedB, lr: "B2"},
			// Beyond the replacementAttempts limit: {d: digestCompressedA, lr: "A2"},
		}, cache.CandidateLocations(transport, scope, digestUncompressed, true))
		res = cache.CandidateLocations2(transport, scope, digestUncompressed, blobinfocache.CandidateLocations2Options{
			CanSubstitute: true,
		})
		assertCandidatesMatch2(t, scopeName, []candidate{
			{d: digestUncompressed, cn: compressorNameU, lr: "U1"}, {d: digestUncompressed, cn: compressorNameU, lr: "U2"},
			// "1" entries were added after "2", and A/Bs are sorted in the reverse of digestNameSetPrioritization order
			{d: digestCompressedB, cn: compressorNameB, lr: "B1"},
			{d: digestCompressedA, cn: compressorNameA, lr: "A1"},
			{d: digestCompressedB, cn: compressorNameB, lr: "B2"},
			// Beyond the replacementAttempts limit: {d: digestCompressedA, cn: compressorNameA, lr: "A2"},
		}, res)

		// Locations are known, but no relationships
		assertCandidatesMatch(t, scopeName, []candidate{
			{d: digestCompressedUnrelated, lr: "CU1"}, {d: digestCompressedUnrelated, lr: "CU2"},
		}, cache.CandidateLocations(transport, scope, digestCompressedUnrelated, true))
		res = cache.CandidateLocations2(transport, scope, digestCompressedUnrelated, blobinfocache.CandidateLocations2Options{
			CanSubstitute: true,
		})
		assertCandidatesMatch2(t, scopeName, []candidate{
			{d: digestCompressedUnrelated, cn: compressorNameCU, lr: "CU1"}, {d: digestCompressedUnrelated, cn: compressorNameCU, lr: "CU2"},
		}, res)

		// Tests of candidate filtering
		// ----------------------------
		for _, e := range digestNameSetFiltering {
			cache.RecordKnownLocation(transport, scope, e.d, types.BICLocationReference{Opaque: scopeName + e.n})
		}
		for _, e := range digestNameSetFiltering {
			cache.RecordDigestCompressorData(e.d, blobinfocache.DigestCompressorData{
				BaseVariantCompressor:      e.base,
				SpecificVariantCompressor:  e.specific,
				SpecificVariantAnnotations: e.annotations,
			})
		}

		// No filtering
		res = cache.CandidateLocations2(transport, scope, digestFilteringUncompressed, blobinfocache.CandidateLocations2Options{
			CanSubstitute: true,
		})
		assertCandidatesMatch2(t, scopeName, []candidate{ // Sorted in the reverse of digestNameSetFiltering order
			{d: digestZstdChunked, cn: compressiontypes.ZstdChunkedAlgorithmName, ca: chunkedAnnotations, lr: "zstdChunked"},
			{d: digestZstd, cn: compressiontypes.ZstdAlgorithmName, ca: nil, lr: "zstd"},
			{d: digestGzip, cn: compressiontypes.GzipAlgorithmName, ca: nil, lr: "gzip"},
		}, res)

		// Manifest format filters
		res = cache.CandidateLocations2(transport, scope, digestFilteringUncompressed, blobinfocache.CandidateLocations2Options{
			CanSubstitute:           true,
			PossibleManifestFormats: []string{manifest.DockerV2Schema2MediaType},
		})
		assertCandidatesMatch2(t, scopeName, []candidate{
			{d: digestGzip, cn: compressiontypes.GzipAlgorithmName, ca: nil, lr: "gzip"},
		}, res)
		res = cache.CandidateLocations2(transport, scope, digestFilteringUncompressed, blobinfocache.CandidateLocations2Options{
			CanSubstitute:           true,
			PossibleManifestFormats: []string{imgspecv1.MediaTypeImageManifest},
		})
		assertCandidatesMatch2(t, scopeName, []candidate{ // Sorted in the reverse of digestNameSetFiltering order
			{d: digestZstdChunked, cn: compressiontypes.ZstdChunkedAlgorithmName, ca: chunkedAnnotations, lr: "zstdChunked"},
			{d: digestZstd, cn: compressiontypes.ZstdAlgorithmName, ca: nil, lr: "zstd"},
			{d: digestGzip, cn: compressiontypes.GzipAlgorithmName, ca: nil, lr: "gzip"},
		}, res)

		// Compression algorithm filters
		res = cache.CandidateLocations2(transport, scope, digestFilteringUncompressed, blobinfocache.CandidateLocations2Options{
			CanSubstitute:       true,
			RequiredCompression: &compression.Gzip,
		})
		assertCandidatesMatch2(t, scopeName, []candidate{
			{d: digestGzip, cn: compressiontypes.GzipAlgorithmName, ca: nil, lr: "gzip"},
		}, res)
		res = cache.CandidateLocations2(transport, scope, digestFilteringUncompressed, blobinfocache.CandidateLocations2Options{
			CanSubstitute:       true,
			RequiredCompression: &compression.ZstdChunked,
		})
		assertCandidatesMatch2(t, scopeName, []candidate{
			{d: digestZstdChunked, cn: compressiontypes.ZstdChunkedAlgorithmName, ca: chunkedAnnotations, lr: "zstdChunked"},
		}, res)
		res = cache.CandidateLocations2(transport, scope, digestFilteringUncompressed, blobinfocache.CandidateLocations2Options{
			CanSubstitute:       true,
			RequiredCompression: &compression.Zstd,
		})
		assertCandidatesMatch2(t, scopeName, []candidate{ // When the user asks for zstd, zstd:chunked candidates are also acceptable, and include the chunked information.
			{d: digestZstdChunked, cn: compressiontypes.ZstdChunkedAlgorithmName, ca: chunkedAnnotations, lr: "zstdChunked"},
			{d: digestZstd, cn: compressiontypes.ZstdAlgorithmName, ca: nil, lr: "zstd"},
		}, res)

		// After RecordDigestCompressorData with zstd:chunked details, a later call with zstd-only does not drop the chunked details.
		cache.RecordDigestCompressorData(digestZstdChunked, blobinfocache.DigestCompressorData{
			BaseVariantCompressor:      compressiontypes.ZstdAlgorithmName,
			SpecificVariantCompressor:  blobinfocache.UnknownCompression,
			SpecificVariantAnnotations: nil,
		})
		res = cache.CandidateLocations2(transport, scope, digestFilteringUncompressed, blobinfocache.CandidateLocations2Options{
			CanSubstitute: true,
		})
		assertCandidatesMatch2(t, scopeName, []candidate{ // Sorted in the reverse of digestNameSetFiltering order
			{d: digestZstdChunked, cn: compressiontypes.ZstdChunkedAlgorithmName, ca: chunkedAnnotations, lr: "zstdChunked"},
			{d: digestZstd, cn: compressiontypes.ZstdAlgorithmName, ca: nil, lr: "zstd"},
			{d: digestGzip, cn: compressiontypes.GzipAlgorithmName, ca: nil, lr: "gzip"},
		}, res)
	}
}

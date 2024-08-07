package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/containers/image/v5/pkg/compression"
	digest "github.com/opencontainers/go-digest"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	digestSha256EmptyTar = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
)

func TestGuessMIMEType(t *testing.T) {
	cases := []struct {
		path     string
		mimeType string
	}{
		{"v2s2.manifest.json", DockerV2Schema2MediaType},
		{"v2list.manifest.json", DockerV2ListMediaType},
		{"v2s1.manifest.json", DockerV2Schema1SignedMediaType},
		{"v2s1-unsigned.manifest.json", DockerV2Schema1MediaType},
		{"v2s1-invalid-signatures.manifest.json", DockerV2Schema1SignedMediaType},
		{"v2s2nomime.manifest.json", DockerV2Schema2MediaType}, // It is unclear whether this one is legal, but we should guess v2s2 if anything at all.
		{"unknown-version.manifest.json", ""},
		{"non-json.manifest.json", ""}, // Not a manifest (nor JSON) at all
		{"ociv1.manifest.json", imgspecv1.MediaTypeImageManifest},
		{"ociv1.artifact.json", imgspecv1.MediaTypeImageManifest},
		{"ociv1.image.index.json", imgspecv1.MediaTypeImageIndex},
		{"ociv1nomime.manifest.json", imgspecv1.MediaTypeImageManifest},
		{"ociv1nomime.artifact.json", imgspecv1.MediaTypeImageManifest},
		{"ociv1nomime.image.index.json", imgspecv1.MediaTypeImageIndex},
	}

	for _, c := range cases {
		manifest, err := os.ReadFile(filepath.Join("testdata", c.path))
		require.NoError(t, err)
		mimeType := GuessMIMEType(manifest)
		assert.Equal(t, c.mimeType, mimeType, c.path)
	}
}

func TestDigest(t *testing.T) {
	cases := []struct {
		path           string
		expectedDigest digest.Digest
	}{
		{"v2s2.manifest.json", TestDockerV2S2ManifestDigest},
		{"v2s1.manifest.json", TestDockerV2S1ManifestDigest},
		{"v2s1-unsigned.manifest.json", TestDockerV2S1UnsignedManifestDigest},
	}
	for _, c := range cases {
		manifest, err := os.ReadFile(filepath.Join("testdata", c.path))
		require.NoError(t, err)
		actualDigest, err := Digest(manifest)
		require.NoError(t, err)
		assert.Equal(t, c.expectedDigest, actualDigest)
	}

	manifest, err := os.ReadFile("testdata/v2s1-invalid-signatures.manifest.json")
	require.NoError(t, err)
	_, err = Digest(manifest)
	assert.Error(t, err)

	actualDigest, err := Digest([]byte{})
	require.NoError(t, err)
	assert.Equal(t, digest.Digest(digestSha256EmptyTar), actualDigest)
}

func TestMatchesDigest(t *testing.T) {
	cases := []struct {
		path           string
		expectedDigest digest.Digest
		result         bool
	}{
		// Success
		{"v2s2.manifest.json", TestDockerV2S2ManifestDigest, true},
		{"v2s1.manifest.json", TestDockerV2S1ManifestDigest, true},
		// No match (switched s1/s2)
		{"v2s2.manifest.json", TestDockerV2S1ManifestDigest, false},
		{"v2s1.manifest.json", TestDockerV2S2ManifestDigest, false},
		// Unrecognized algorithm
		{"v2s2.manifest.json", digest.Digest("md5:2872f31c5c1f62a694fbd20c1e85257c"), false},
		// Mangled format
		{"v2s2.manifest.json", digest.Digest(TestDockerV2S2ManifestDigest.String() + "abc"), false},
		{"v2s2.manifest.json", digest.Digest(TestDockerV2S2ManifestDigest.String()[:20]), false},
		{"v2s2.manifest.json", digest.Digest(""), false},
	}
	for _, c := range cases {
		manifest, err := os.ReadFile(filepath.Join("testdata", c.path))
		require.NoError(t, err)
		res, err := MatchesDigest(manifest, c.expectedDigest)
		require.NoError(t, err)
		assert.Equal(t, c.result, res)
	}

	manifest, err := os.ReadFile("testdata/v2s1-invalid-signatures.manifest.json")
	require.NoError(t, err)
	// Even a correct SHA256 hash is rejected if we can't strip the JSON signature.
	res, err := MatchesDigest(manifest, digest.FromBytes(manifest))
	assert.False(t, res)
	assert.Error(t, err)

	res, err = MatchesDigest([]byte{}, digest.Digest(digestSha256EmptyTar))
	assert.True(t, res)
	assert.NoError(t, err)
}

func TestNormalizedMIMEType(t *testing.T) {
	for _, c := range []string{ // Valid MIME types, normalized to themselves
		DockerV2Schema1MediaType,
		DockerV2Schema1SignedMediaType,
		DockerV2Schema2MediaType,
		DockerV2ListMediaType,
		imgspecv1.MediaTypeImageManifest,
		imgspecv1.MediaTypeImageIndex,
	} {
		res := NormalizedMIMEType(c)
		assert.Equal(t, c, res, c)
	}
	for _, c := range []string{
		"application/json",
		"text/plain",
		"not at all a valid MIME type",
		"",
	} {
		res := NormalizedMIMEType(c)
		assert.Equal(t, DockerV2Schema1SignedMediaType, res, c)
	}
}

func TestCompressionAlgorithmIsUniversallySupported(t *testing.T) {
	for _, algo := range []compression.Algorithm{compression.Gzip} {
		res := CompressionAlgorithmIsUniversallySupported(algo)
		assert.True(t, res, algo.Name())
	}

	for _, algo := range []compression.Algorithm{
		compression.Bzip2,
		compression.Xz,
		compression.Zstd,
		compression.ZstdChunked,
	} {
		res := CompressionAlgorithmIsUniversallySupported(algo)
		assert.False(t, res, algo.Name())
	}
}

func TestMIMETypeSupportsCompressionAlgorithm(t *testing.T) {
	allMIMETypes := []string{imgspecv1.MediaTypeImageManifest, DockerV2Schema2MediaType, DockerV2Schema1SignedMediaType, DockerV2Schema1MediaType}

	for _, algo := range []compression.Algorithm{compression.Gzip} {
		for _, mt := range allMIMETypes {
			res := MIMETypeSupportsCompressionAlgorithm(mt, algo)
			assert.True(t, res, fmt.Sprintf("%s, %s", mt, algo.Name()))
		}
	}

	for _, algo := range []compression.Algorithm{
		compression.Bzip2,
		compression.Xz,
	} {
		for _, mt := range allMIMETypes {
			res := MIMETypeSupportsCompressionAlgorithm(mt, algo)
			assert.False(t, res, fmt.Sprintf("%s, %s", mt, algo.Name()))
		}
	}

	for _, algo := range []compression.Algorithm{
		compression.Zstd,
		compression.ZstdChunked,
	} {
		for _, mt := range allMIMETypes {
			res := MIMETypeSupportsCompressionAlgorithm(mt, algo)
			assert.Equal(t, mt == imgspecv1.MediaTypeImageManifest, res, fmt.Sprintf("%s, %s", mt, algo.Name()))
		}
	}
}

func TestCandidateCompressionMatchesReuseConditions(t *testing.T) {
	cases := []struct {
		requiredCompression     *compression.Algorithm
		possibleManifestFormats []string
		candidateCompression    *compression.Algorithm
		result                  bool
	}{
		// RequiredCompression restrictions
		{&compression.Zstd, nil, &compression.Zstd, true},
		{&compression.Zstd, nil, &compression.ZstdChunked, true},
		{&compression.ZstdChunked, nil, &compression.Zstd, false},
		{&compression.ZstdChunked, nil, &compression.ZstdChunked, true},
		{&compression.Gzip, nil, &compression.Zstd, false},
		{&compression.Zstd, nil, nil, false},
		{nil, nil, &compression.Zstd, true},
		// PossibleManifestFormats restrictions
		{nil, []string{imgspecv1.MediaTypeImageManifest}, &compression.Zstd, true},
		{nil, []string{DockerV2Schema2MediaType}, &compression.Zstd, false},
		{nil, []string{DockerV2Schema2MediaType, DockerV2Schema1SignedMediaType, imgspecv1.MediaTypeImageManifest}, &compression.Zstd, true},
		{nil, nil, &compression.Zstd, true},
		{nil, []string{imgspecv1.MediaTypeImageManifest}, &compression.Gzip, true},
		{nil, []string{DockerV2Schema2MediaType}, &compression.Gzip, true},
		{nil, []string{DockerV2Schema2MediaType, DockerV2Schema1SignedMediaType, imgspecv1.MediaTypeImageManifest}, &compression.Gzip, true},
		{nil, nil, &compression.Gzip, true},
		// Some possible combinations (always 1 constraint not matching)
		{&compression.Zstd, []string{DockerV2Schema2MediaType}, &compression.Zstd, false},
		{&compression.Gzip, []string{DockerV2Schema2MediaType, DockerV2Schema1SignedMediaType, imgspecv1.MediaTypeImageManifest}, &compression.Zstd, false},
	}

	for _, c := range cases {
		conds := ReuseConditions{
			RequiredCompression:     c.requiredCompression,
			PossibleManifestFormats: c.possibleManifestFormats,
		}
		assert.Equal(t, c.result, CandidateCompressionMatchesReuseConditions(conds, c.candidateCompression))
	}
}

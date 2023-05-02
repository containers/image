package copy

import (
	"io/ioutil"
	"strings"
	"testing"

	internalManifest "github.com/containers/image/v5/internal/manifest"
	digest "github.com/opencontainers/go-digest"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetermineSpecificImages(t *testing.T) {
	testCases := []struct {
		id                  string
		fixture             string
		instanceDigests     []digest.Digest
		instancePlatforms   []imgspecv1.Platform
		expected            []digest.Digest
		expectedErrIncludes string
	}{
		{
			id:      "no inputs no outputs",
			fixture: "../manifest/fixtures/v2list.manifest.json",
		},
		{
			id:      "instances only out of order",
			fixture: "../manifest/fixtures/v2list.manifest.json",
			instanceDigests: []digest.Digest{
				"sha256:e4c0df75810b953d6717b8f8f28298d73870e8aa2a0d5e77b8391f16fdfbbbe2",
				"sha256:7820f9a86d4ad15a2c4f0c0e5479298df2aa7c2f6871288e2ef8546f3e7b6783",
			},
			expected: []digest.Digest{
				"sha256:7820f9a86d4ad15a2c4f0c0e5479298df2aa7c2f6871288e2ef8546f3e7b6783",
				"sha256:e4c0df75810b953d6717b8f8f28298d73870e8aa2a0d5e77b8391f16fdfbbbe2",
			},
		},
		{
			id:      "instances only in order",
			fixture: "../manifest/fixtures/v2list.manifest.json",
			instanceDigests: []digest.Digest{
				"sha256:7820f9a86d4ad15a2c4f0c0e5479298df2aa7c2f6871288e2ef8546f3e7b6783",
				"sha256:e4c0df75810b953d6717b8f8f28298d73870e8aa2a0d5e77b8391f16fdfbbbe2",
			},
			expected: []digest.Digest{
				"sha256:7820f9a86d4ad15a2c4f0c0e5479298df2aa7c2f6871288e2ef8546f3e7b6783",
				"sha256:e4c0df75810b953d6717b8f8f28298d73870e8aa2a0d5e77b8391f16fdfbbbe2",
			},
		},
		{
			id:      "platforms only in order",
			fixture: "../manifest/fixtures/v2list.manifest.json",
			instancePlatforms: []imgspecv1.Platform{
				{
					OS:           "linux",
					Architecture: "ppc64le",
				},
				{
					OS:           "linux",
					Architecture: "s390x",
				},
			},
			expected: []digest.Digest{
				"sha256:7820f9a86d4ad15a2c4f0c0e5479298df2aa7c2f6871288e2ef8546f3e7b6783",
				"sha256:e4c0df75810b953d6717b8f8f28298d73870e8aa2a0d5e77b8391f16fdfbbbe2",
			},
		},
		{
			id:      "platforms only out of order",
			fixture: "../manifest/fixtures/v2list.manifest.json",
			instancePlatforms: []imgspecv1.Platform{
				{
					OS:           "linux",
					Architecture: "s390x",
				},
				{
					OS:           "linux",
					Architecture: "ppc64le",
				},
			},
			expected: []digest.Digest{
				"sha256:7820f9a86d4ad15a2c4f0c0e5479298df2aa7c2f6871288e2ef8546f3e7b6783",
				"sha256:e4c0df75810b953d6717b8f8f28298d73870e8aa2a0d5e77b8391f16fdfbbbe2",
			},
		},
		{
			id:      "mixed without duplicates",
			fixture: "../manifest/fixtures/v2list.manifest.json",
			instancePlatforms: []imgspecv1.Platform{
				{
					OS:           "linux",
					Architecture: "s390x",
				},
			},
			instanceDigests: []digest.Digest{
				"sha256:7820f9a86d4ad15a2c4f0c0e5479298df2aa7c2f6871288e2ef8546f3e7b6783",
			},
			expected: []digest.Digest{
				"sha256:7820f9a86d4ad15a2c4f0c0e5479298df2aa7c2f6871288e2ef8546f3e7b6783",
				"sha256:e4c0df75810b953d6717b8f8f28298d73870e8aa2a0d5e77b8391f16fdfbbbe2",
			},
		},
		{
			id:      "mixed with duplicates",
			fixture: "../manifest/fixtures/v2list.manifest.json",
			instancePlatforms: []imgspecv1.Platform{
				{
					OS:           "linux",
					Architecture: "ppc64le",
				},
				{
					OS:           "linux",
					Architecture: "s390x",
				},
			},
			instanceDigests: []digest.Digest{
				"sha256:7820f9a86d4ad15a2c4f0c0e5479298df2aa7c2f6871288e2ef8546f3e7b6783",
				"sha256:e4c0df75810b953d6717b8f8f28298d73870e8aa2a0d5e77b8391f16fdfbbbe2",
			},
			expected: []digest.Digest{
				"sha256:7820f9a86d4ad15a2c4f0c0e5479298df2aa7c2f6871288e2ef8546f3e7b6783",
				"sha256:e4c0df75810b953d6717b8f8f28298d73870e8aa2a0d5e77b8391f16fdfbbbe2",
			},
		},
		{
			id:      "no such platform",
			fixture: "../manifest/fixtures/v2list.manifest.json",
			instancePlatforms: []imgspecv1.Platform{
				{
					OS:           "windows",
					Architecture: "amd64",
				},
				{
					OS:           "darwin",
					Architecture: "arm64",
				},
			},
			expectedErrIncludes: "no image found in manifest list for",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.id, func(t *testing.T) {
			listBlob, err := ioutil.ReadFile(tc.fixture)
			require.NoErrorf(t, err, "unexpected error reading fixture %q", tc.fixture)
			list, err := internalManifest.ListFromBlob(listBlob, internalManifest.GuessMIMEType(listBlob))
			require.NoErrorf(t, err, "unexpected error parsing fixture %q", tc.fixture)
			options := Options{
				Instances:         tc.instanceDigests,
				InstancePlatforms: tc.instancePlatforms,
			}
			specific, err := determineSpecificImages(&options, list)
			if err != nil && tc.expectedErrIncludes != "" {
				if strings.Contains(err.Error(), tc.expectedErrIncludes) {
					// okay
					return
				}
			}
			require.NoErrorf(t, err, "unexpected error selecting instances")
			var selected []digest.Digest
			for _, instanceDigest := range list.Instances() {
				if specific.Contains(instanceDigest) {
					selected = append(selected, instanceDigest)
				}
			}
			assert.Equalf(t, tc.expected, selected, "given instance list %#v and platforms list %#v", tc.instanceDigests, tc.instancePlatforms)
		})
	}
}

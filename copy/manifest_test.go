package copy

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/containers/image/v5/internal/testing/mocks"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/pkg/compression"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrderedSet(t *testing.T) {
	for _, c := range []struct{ input, expected []string }{
		{[]string{}, []string{}},
		{[]string{"a", "b", "c"}, []string{"a", "b", "c"}},
		{[]string{"a", "b", "a", "c"}, []string{"a", "b", "c"}},
	} {
		os := newOrderedSet()
		for _, s := range c.input {
			os.append(s)
		}
		assert.Equal(t, c.expected, os.list, fmt.Sprintf("%#v", c.input))
	}
}

func TestDetermineManifestConversion(t *testing.T) {
	supportS1S2OCI := []string{
		v1.MediaTypeImageManifest,
		manifest.DockerV2Schema2MediaType,
		manifest.DockerV2Schema1SignedMediaType,
		manifest.DockerV2Schema1MediaType,
	}
	supportS1OCI := []string{
		v1.MediaTypeImageManifest,
		manifest.DockerV2Schema1SignedMediaType,
		manifest.DockerV2Schema1MediaType,
	}
	supportS1S2 := []string{
		manifest.DockerV2Schema2MediaType,
		manifest.DockerV2Schema1SignedMediaType,
		manifest.DockerV2Schema1MediaType,
	}
	supportOnlyS1 := []string{
		manifest.DockerV2Schema1SignedMediaType,
		manifest.DockerV2Schema1MediaType,
	}

	cases := []struct {
		description string
		sourceType  string
		destTypes   []string
		expected    manifestConversionPlan
	}{
		// Destination accepts anything — consider all options, prefer the source format
		{
			"s1→anything", manifest.DockerV2Schema1SignedMediaType, nil,
			manifestConversionPlan{
				preferredMIMEType:                manifest.DockerV2Schema1SignedMediaType,
				preferredMIMETypeNeedsConversion: false,
				otherMIMETypeCandidates:          []string{manifest.DockerV2Schema2MediaType, v1.MediaTypeImageManifest, manifest.DockerV2Schema1MediaType},
			},
		},
		{
			"s2→anything", manifest.DockerV2Schema2MediaType, nil,
			manifestConversionPlan{
				preferredMIMEType:                manifest.DockerV2Schema2MediaType,
				preferredMIMETypeNeedsConversion: false,
				otherMIMETypeCandidates:          []string{manifest.DockerV2Schema1SignedMediaType, v1.MediaTypeImageManifest, manifest.DockerV2Schema1MediaType},
			},
		},
		// Destination accepts the unmodified original
		{
			"s1→s1s2", manifest.DockerV2Schema1SignedMediaType, supportS1S2,
			manifestConversionPlan{
				preferredMIMEType:                manifest.DockerV2Schema1SignedMediaType,
				preferredMIMETypeNeedsConversion: false,
				otherMIMETypeCandidates:          []string{manifest.DockerV2Schema2MediaType, manifest.DockerV2Schema1MediaType},
			},
		},
		{
			"s2→s1s2", manifest.DockerV2Schema2MediaType, supportS1S2,
			manifestConversionPlan{
				preferredMIMEType:                manifest.DockerV2Schema2MediaType,
				preferredMIMETypeNeedsConversion: false,
				otherMIMETypeCandidates:          supportOnlyS1,
			},
		},
		{
			"s1→s1", manifest.DockerV2Schema1SignedMediaType, supportOnlyS1,
			manifestConversionPlan{
				preferredMIMEType:                manifest.DockerV2Schema1SignedMediaType,
				preferredMIMETypeNeedsConversion: false,
				otherMIMETypeCandidates:          []string{manifest.DockerV2Schema1MediaType},
			},
		},
		// text/plain is normalized to s1, and if the destination accepts s1, no conversion happens.
		{
			"text→s1s2", "text/plain", supportS1S2,
			manifestConversionPlan{
				preferredMIMEType:                manifest.DockerV2Schema1SignedMediaType,
				preferredMIMETypeNeedsConversion: false,
				otherMIMETypeCandidates:          []string{manifest.DockerV2Schema2MediaType, manifest.DockerV2Schema1MediaType},
			},
		},
		{
			"text→s1", "text/plain", supportOnlyS1,
			manifestConversionPlan{
				preferredMIMEType:                manifest.DockerV2Schema1SignedMediaType,
				preferredMIMETypeNeedsConversion: false,
				otherMIMETypeCandidates:          []string{manifest.DockerV2Schema1MediaType},
			},
		},
		// Conversion necessary, a preferred format is acceptable
		{
			"s2→s1", manifest.DockerV2Schema2MediaType, supportOnlyS1,
			manifestConversionPlan{
				preferredMIMEType:                manifest.DockerV2Schema1SignedMediaType,
				preferredMIMETypeNeedsConversion: true,
				otherMIMETypeCandidates:          []string{manifest.DockerV2Schema1MediaType},
			},
		},
		// Conversion necessary, a preferred format is not acceptable
		{
			"s2→OCI", manifest.DockerV2Schema2MediaType, []string{v1.MediaTypeImageManifest},
			manifestConversionPlan{
				preferredMIMEType:                v1.MediaTypeImageManifest,
				preferredMIMETypeNeedsConversion: true,
				otherMIMETypeCandidates:          []string{},
			},
		},
		// text/plain is converted if the destination does not accept s1
		{
			"text→s2", "text/plain", []string{manifest.DockerV2Schema2MediaType},
			manifestConversionPlan{
				preferredMIMEType:                manifest.DockerV2Schema2MediaType,
				preferredMIMETypeNeedsConversion: true,
				otherMIMETypeCandidates:          []string{},
			},
		},
		// Conversion necessary, try the preferred formats in order.
		// We abuse manifest.DockerV2ListMediaType here as a MIME type which is not in supportS1S2OCI,
		// but is still recognized by manifest.NormalizedMIMEType and not normalized to s1
		{
			"special→s2", manifest.DockerV2ListMediaType, supportS1S2OCI,
			manifestConversionPlan{
				preferredMIMEType:                manifest.DockerV2Schema2MediaType,
				preferredMIMETypeNeedsConversion: true,
				otherMIMETypeCandidates:          []string{manifest.DockerV2Schema1SignedMediaType, v1.MediaTypeImageManifest, manifest.DockerV2Schema1MediaType},
			},
		},
		{
			"special→s1", manifest.DockerV2ListMediaType, supportS1OCI,
			manifestConversionPlan{
				preferredMIMEType:                manifest.DockerV2Schema1SignedMediaType,
				preferredMIMETypeNeedsConversion: true,
				otherMIMETypeCandidates:          []string{v1.MediaTypeImageManifest, manifest.DockerV2Schema1MediaType},
			},
		},
		{
			"special→OCI", manifest.DockerV2ListMediaType, []string{v1.MediaTypeImageManifest, "other options", "with lower priority"},
			manifestConversionPlan{
				preferredMIMEType:                v1.MediaTypeImageManifest,
				preferredMIMETypeNeedsConversion: true,
				otherMIMETypeCandidates:          []string{"other options", "with lower priority"},
			},
		},
	}

	for _, c := range cases {
		res, err := determineManifestConversion(determineManifestConversionInputs{
			srcMIMEType:                    c.sourceType,
			destSupportedManifestMIMETypes: c.destTypes,
			forceManifestMIMEType:          "",
			requiresOCIEncryption:          false,
			cannotModifyManifestReason:     "",
		})
		require.NoError(t, err, c.description)
		assert.Equal(t, c.expected, res, c.description)
	}

	// Whatever the input is, with cannotModifyManifestReason we return "keep the original as is"
	for _, c := range cases {
		res, err := determineManifestConversion(determineManifestConversionInputs{
			srcMIMEType:                    c.sourceType,
			destSupportedManifestMIMETypes: c.destTypes,
			forceManifestMIMEType:          "",
			requiresOCIEncryption:          false,
			cannotModifyManifestReason:     "Preserving digests",
		})
		require.NoError(t, err, c.description)
		assert.Equal(t, manifestConversionPlan{
			preferredMIMEType:                manifest.NormalizedMIMEType(c.sourceType),
			preferredMIMETypeNeedsConversion: false,
			otherMIMETypeCandidates:          []string{},
		}, res, c.description)
	}

	// With forceManifestMIMEType, the output is always the forced manifest type (in this case oci manifest)
	for _, c := range cases {
		res, err := determineManifestConversion(determineManifestConversionInputs{
			srcMIMEType:                    c.sourceType,
			destSupportedManifestMIMETypes: c.destTypes,
			forceManifestMIMEType:          v1.MediaTypeImageManifest,
			requiresOCIEncryption:          false,
			cannotModifyManifestReason:     "",
		})
		require.NoError(t, err, c.description)
		assert.Equal(t, manifestConversionPlan{
			preferredMIMEType:                v1.MediaTypeImageManifest,
			preferredMIMETypeNeedsConversion: true,
			otherMIMETypeCandidates:          []string{},
		}, res, c.description)
	}

	// When encryption or zstd is required:
	// In both of these cases, we we are restricted to OCI
	for _, c := range []struct {
		description string
		in          determineManifestConversionInputs // with requiresOCIEncryption or requestedCompressionFormat: zstd implied
		expected    manifestConversionPlan            // Or {} to expect a failure
	}{
		{ // Destination accepts anything - no conversion necessary
			"OCI→anything",
			determineManifestConversionInputs{
				srcMIMEType:                    v1.MediaTypeImageManifest,
				destSupportedManifestMIMETypes: nil,
			},
			manifestConversionPlan{
				preferredMIMEType:                v1.MediaTypeImageManifest,
				preferredMIMETypeNeedsConversion: false,
				otherMIMETypeCandidates:          []string{},
			},
		},
		{ // Destination accepts anything - need to convert to OCI
			"s2→anything",
			determineManifestConversionInputs{
				srcMIMEType:                    manifest.DockerV2Schema2MediaType,
				destSupportedManifestMIMETypes: nil,
			},
			manifestConversionPlan{
				preferredMIMEType:                v1.MediaTypeImageManifest,
				preferredMIMETypeNeedsConversion: true,
				otherMIMETypeCandidates:          []string{},
			},
		},
		// Destination accepts OCI
		{
			"OCI→OCI",
			determineManifestConversionInputs{
				srcMIMEType:                    v1.MediaTypeImageManifest,
				destSupportedManifestMIMETypes: supportS1S2OCI,
			},
			manifestConversionPlan{
				preferredMIMEType:                v1.MediaTypeImageManifest,
				preferredMIMETypeNeedsConversion: false,
				otherMIMETypeCandidates:          []string{},
			},
		},
		{
			"s2→OCI",
			determineManifestConversionInputs{
				srcMIMEType:                    manifest.DockerV2Schema2MediaType,
				destSupportedManifestMIMETypes: supportS1S2OCI,
			},
			manifestConversionPlan{
				preferredMIMEType:                v1.MediaTypeImageManifest,
				preferredMIMETypeNeedsConversion: true,
				otherMIMETypeCandidates:          []string{},
			},
		},
		// Destination does not accept OCI
		{
			"OCI→s2",
			determineManifestConversionInputs{
				srcMIMEType:                    v1.MediaTypeImageManifest,
				destSupportedManifestMIMETypes: supportS1S2,
			},
			manifestConversionPlan{},
		},
		{
			"s2→s2",
			determineManifestConversionInputs{
				srcMIMEType:                    manifest.DockerV2Schema2MediaType,
				destSupportedManifestMIMETypes: supportS1S2,
			},
			manifestConversionPlan{},
		},
		// Whatever the input is, with cannotModifyManifestReason we return "keep the original as is".
		// Still, encryption/compression is necessarily going to fail…
		{
			"OCI cannotModifyManifestReason",
			determineManifestConversionInputs{
				srcMIMEType:                    v1.MediaTypeImageManifest,
				destSupportedManifestMIMETypes: supportS1S2OCI,
				cannotModifyManifestReason:     "Preserving digests",
			},
			manifestConversionPlan{
				preferredMIMEType:                v1.MediaTypeImageManifest,
				preferredMIMETypeNeedsConversion: false,
				otherMIMETypeCandidates:          []string{},
			},
		},
		{
			"s2 cannotModifyManifestReason",
			determineManifestConversionInputs{
				srcMIMEType:                    manifest.DockerV2Schema2MediaType,
				destSupportedManifestMIMETypes: supportS1S2OCI,
				cannotModifyManifestReason:     "Preserving digests",
			},
			manifestConversionPlan{
				preferredMIMEType:                manifest.DockerV2Schema2MediaType,
				preferredMIMETypeNeedsConversion: false,
				otherMIMETypeCandidates:          []string{},
			},
		},
		// forceManifestMIMEType to a type that supports OCI features
		{
			"OCI→OCI forced",
			determineManifestConversionInputs{
				srcMIMEType:                    v1.MediaTypeImageManifest,
				destSupportedManifestMIMETypes: supportS1S2OCI,
				forceManifestMIMEType:          v1.MediaTypeImageManifest,
			},
			manifestConversionPlan{
				preferredMIMEType:                v1.MediaTypeImageManifest,
				preferredMIMETypeNeedsConversion: false,
				otherMIMETypeCandidates:          []string{},
			},
		},
		{
			"s2→OCI forced",
			determineManifestConversionInputs{
				srcMIMEType:                    manifest.DockerV2Schema2MediaType,
				destSupportedManifestMIMETypes: supportS1S2OCI,
				forceManifestMIMEType:          v1.MediaTypeImageManifest,
			},
			manifestConversionPlan{
				preferredMIMEType:                v1.MediaTypeImageManifest,
				preferredMIMETypeNeedsConversion: true,
				otherMIMETypeCandidates:          []string{},
			},
		},
		// forceManifestMIMEType to a type that does not support OCI features
		{
			"OCI→s2 forced",
			determineManifestConversionInputs{
				srcMIMEType:                    v1.MediaTypeImageManifest,
				destSupportedManifestMIMETypes: supportS1S2OCI,
				forceManifestMIMEType:          manifest.DockerV2Schema2MediaType,
			},
			manifestConversionPlan{},
		},
		{
			"s2→s2 forced",
			determineManifestConversionInputs{
				srcMIMEType:                    manifest.DockerV2Schema2MediaType,
				destSupportedManifestMIMETypes: supportS1S2OCI,
				forceManifestMIMEType:          manifest.DockerV2Schema2MediaType,
			},
			manifestConversionPlan{},
		},
	} {
		for _, restriction := range []struct {
			description string
			edit        func(in *determineManifestConversionInputs)
		}{
			{
				description: "encrypted",
				edit: func(in *determineManifestConversionInputs) {
					in.requiresOCIEncryption = true
				},
			},
			{
				description: "zstd",
				edit: func(in *determineManifestConversionInputs) {
					in.requestedCompressionFormat = &compression.Zstd
				},
			},
			{
				description: "zstd:chunked",
				edit: func(in *determineManifestConversionInputs) {
					in.requestedCompressionFormat = &compression.ZstdChunked
				},
			},
			{
				description: "encrypted+zstd",
				edit: func(in *determineManifestConversionInputs) {
					in.requiresOCIEncryption = true
					in.requestedCompressionFormat = &compression.Zstd
				},
			},
		} {
			desc := c.description + " / " + restriction.description

			in := c.in
			restriction.edit(&in)
			res, err := determineManifestConversion(in)
			if c.expected.preferredMIMEType != "" {
				require.NoError(t, err, desc)
				assert.Equal(t, c.expected, res, desc)
			} else {
				assert.Error(t, err, desc)
			}
		}
	}

	// When encryption using a completely unsupported algorithm is required:
	for _, c := range []struct {
		description string
		in          determineManifestConversionInputs // with requiresOCIEncryption or requestedCompressionFormat: zstd implied
	}{
		{ // Destination accepts anything
			"OCI→anything",
			determineManifestConversionInputs{
				srcMIMEType:                    v1.MediaTypeImageManifest,
				destSupportedManifestMIMETypes: nil,
			},
		},
		{ // Destination accepts anything - need to convert to OCI
			"s2→anything",
			determineManifestConversionInputs{
				srcMIMEType:                    manifest.DockerV2Schema2MediaType,
				destSupportedManifestMIMETypes: nil,
			},
		},
		// Destination only supports some formats
		{
			"OCI→OCI",
			determineManifestConversionInputs{
				srcMIMEType:                    v1.MediaTypeImageManifest,
				destSupportedManifestMIMETypes: supportS1S2OCI,
			},
		},
		{
			"s2→OCI",
			determineManifestConversionInputs{
				srcMIMEType:                    manifest.DockerV2Schema2MediaType,
				destSupportedManifestMIMETypes: supportS1S2OCI,
			},
		},
		{
			"OCI→s2",
			determineManifestConversionInputs{
				srcMIMEType:                    v1.MediaTypeImageManifest,
				destSupportedManifestMIMETypes: supportS1S2,
			},
		},
		{
			"s2→s2",
			determineManifestConversionInputs{
				srcMIMEType:                    manifest.DockerV2Schema2MediaType,
				destSupportedManifestMIMETypes: supportS1S2,
			},
		},
		// cannotModifyManifestReason
		{
			"OCI cannotModifyManifestReason",
			determineManifestConversionInputs{
				srcMIMEType:                    v1.MediaTypeImageManifest,
				destSupportedManifestMIMETypes: supportS1S2OCI,
				cannotModifyManifestReason:     "Preserving digests",
			},
		},
		{
			"s2 cannotModifyManifestReason",
			determineManifestConversionInputs{
				srcMIMEType:                    manifest.DockerV2Schema2MediaType,
				destSupportedManifestMIMETypes: supportS1S2OCI,
				cannotModifyManifestReason:     "Preserving digests",
			},
		},
		// forceManifestMIMEType
		{
			"OCI→OCI forced",
			determineManifestConversionInputs{
				srcMIMEType:                    v1.MediaTypeImageManifest,
				destSupportedManifestMIMETypes: supportS1S2OCI,
				forceManifestMIMEType:          v1.MediaTypeImageManifest,
			},
		},
		{
			"s2→OCI forced",
			determineManifestConversionInputs{
				srcMIMEType:                    manifest.DockerV2Schema2MediaType,
				destSupportedManifestMIMETypes: supportS1S2OCI,
				forceManifestMIMEType:          v1.MediaTypeImageManifest,
			},
		},
		{
			"OCI→s2 forced",
			determineManifestConversionInputs{
				srcMIMEType:                    v1.MediaTypeImageManifest,
				destSupportedManifestMIMETypes: supportS1S2OCI,
				forceManifestMIMEType:          manifest.DockerV2Schema2MediaType,
			},
		},
		{
			"s2→s2 forced",
			determineManifestConversionInputs{
				srcMIMEType:                    manifest.DockerV2Schema2MediaType,
				destSupportedManifestMIMETypes: supportS1S2OCI,
				forceManifestMIMEType:          manifest.DockerV2Schema2MediaType,
			},
		},
	} {
		in := c.in
		in.requestedCompressionFormat = &compression.Xz
		_, err := determineManifestConversion(in)
		assert.Error(t, err, c.description)
	}
}

// fakeUnparsedImage is an implementation of types.UnparsedImage which only returns itself as a MIME type in Manifest,
// except that "" means “reading the manifest should fail”
type fakeUnparsedImage struct {
	mocks.ForbiddenUnparsedImage
	mt string
}

func (f fakeUnparsedImage) Manifest(ctx context.Context) ([]byte, string, error) {
	if f.mt == "" {
		return nil, "", errors.New("Manifest() directed to fail")
	}
	return nil, f.mt, nil
}

func TestIsMultiImage(t *testing.T) {
	// MIME type is available; more or less a smoke test, other cases are handled in manifest.MIMETypeIsMultiImage
	for _, c := range []struct {
		mt       string
		expected bool
	}{
		{manifest.DockerV2ListMediaType, true},
		{manifest.DockerV2Schema2MediaType, false},
		{v1.MediaTypeImageManifest, false},
		{v1.MediaTypeImageIndex, true},
	} {
		src := fakeUnparsedImage{mocks.ForbiddenUnparsedImage{}, c.mt}
		res, err := isMultiImage(context.Background(), src)
		require.NoError(t, err)
		assert.Equal(t, c.expected, res, c.mt)
	}

	// Error getting manifest MIME type
	src := fakeUnparsedImage{mocks.ForbiddenUnparsedImage{}, ""}
	_, err := isMultiImage(context.Background(), src)
	assert.Error(t, err)
}

func TestDetermineManifestListConversion(t *testing.T) {
	supportS1S2OCI := []string{
		v1.MediaTypeImageIndex,
		v1.MediaTypeImageManifest,
		manifest.DockerV2ListMediaType,
		manifest.DockerV2Schema2MediaType,
		manifest.DockerV2Schema1SignedMediaType,
		manifest.DockerV2Schema1MediaType,
	}
	supportS1S2 := []string{
		manifest.DockerV2ListMediaType,
		manifest.DockerV2Schema2MediaType,
		manifest.DockerV2Schema1SignedMediaType,
		manifest.DockerV2Schema1MediaType,
	}
	supportOnlyOCI := []string{
		v1.MediaTypeImageIndex,
		v1.MediaTypeImageManifest,
	}
	supportOnlyS1 := []string{
		manifest.DockerV2Schema1SignedMediaType,
		manifest.DockerV2Schema1MediaType,
	}

	cases := []struct {
		description             string
		sourceType              string
		destTypes               []string
		expectedUpdate          string
		expectedOtherCandidates []string
	}{
		// Destination accepts anything — try all variants
		{"s2→anything", manifest.DockerV2ListMediaType, nil, "", []string{v1.MediaTypeImageIndex}},
		{"OCI→anything", v1.MediaTypeImageIndex, nil, "", []string{manifest.DockerV2ListMediaType}},
		// Destination accepts the unmodified original
		{"s2→s1s2OCI", manifest.DockerV2ListMediaType, supportS1S2OCI, "", []string{v1.MediaTypeImageIndex}},
		{"OCI→s1s2OCI", v1.MediaTypeImageIndex, supportS1S2OCI, "", []string{manifest.DockerV2ListMediaType}},
		{"s2→s1s2", manifest.DockerV2ListMediaType, supportS1S2, "", []string{}},
		{"OCI→OCI", v1.MediaTypeImageIndex, supportOnlyOCI, "", []string{}},
		// Conversion necessary, try the preferred formats in order.
		{"special→OCI", "unrecognized", supportS1S2OCI, v1.MediaTypeImageIndex, []string{manifest.DockerV2ListMediaType}},
		{"special→s2", "unrecognized", supportS1S2, manifest.DockerV2ListMediaType, []string{}},
	}

	for _, c := range cases {
		copier := &copier{}
		preferredMIMEType, otherCandidates, err := copier.determineListConversion(c.sourceType, c.destTypes, "")
		require.NoError(t, err, c.description)
		if c.expectedUpdate == "" {
			assert.Equal(t, manifest.NormalizedMIMEType(c.sourceType), preferredMIMEType, c.description)
		} else {
			assert.Equal(t, c.expectedUpdate, preferredMIMEType, c.description)
		}
		assert.Equal(t, c.expectedOtherCandidates, otherCandidates, c.description)
	}

	// With forceManifestMIMEType, the output is always the forced manifest type (in this case OCI index)
	for _, c := range cases {
		copier := &copier{}
		preferredMIMEType, otherCandidates, err := copier.determineListConversion(c.sourceType, c.destTypes, v1.MediaTypeImageIndex)
		require.NoError(t, err, c.description)
		assert.Equal(t, v1.MediaTypeImageIndex, preferredMIMEType, c.description)
		assert.Equal(t, []string{}, otherCandidates, c.description)
	}

	// The destination doesn’t support list formats at all
	copier := &copier{}
	_, _, err := copier.determineListConversion(v1.MediaTypeImageIndex, supportOnlyS1, "")
	assert.Error(t, err)
}

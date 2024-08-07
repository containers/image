package signature

import (
	"fmt"
	"testing"

	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/internal/testing/mocks"
	"github.com/containers/image/v5/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	fullRHELRef       = "registry.access.redhat.com/rhel7/rhel:7.2.3"
	untaggedRHELRef   = "registry.access.redhat.com/rhel7/rhel"
	digestSuffix      = "@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	digestSuffixOther = "@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

func TestParseImageAndDockerReference(t *testing.T) {
	const (
		ok1  = "busybox"
		ok2  = fullRHELRef
		bad1 = "UPPERCASE_IS_INVALID_IN_DOCKER_REFERENCES"
		bad2 = ""
	)
	// Success
	ref, err := reference.ParseNormalizedNamed(ok1)
	require.NoError(t, err)
	r1, r2, err := parseImageAndDockerReference(refImageMock{ref: ref}, ok2)
	require.NoError(t, err)
	assert.Equal(t, ok1, reference.FamiliarString(r1))
	assert.Equal(t, ok2, reference.FamiliarString(r2))

	// Unidentified images are rejected.
	_, _, err = parseImageAndDockerReference(refImageMock{ref: nil}, ok2)
	require.Error(t, err)
	assert.IsType(t, PolicyRequirementError(""), err)

	// Failures
	for _, refs := range [][]string{
		{bad1, ok2},
		{ok1, bad2},
		{bad1, bad2},
	} {
		ref, err := reference.ParseNormalizedNamed(refs[0])
		if err == nil {
			_, _, err := parseImageAndDockerReference(refImageMock{ref: ref}, refs[1])
			assert.Error(t, err)
		}
	}
}

// refImageMock is a mock of private.UnparsedImage which returns itself in Reference().DockerReference.
type refImageMock struct {
	mocks.ForbiddenUnparsedImage
	ref reference.Named
}

func (ref refImageMock) Reference() types.ImageReference {
	return refImageReferenceMock{ref: ref.ref}
}

// refImageReferenceMock is a mock of types.ImageReference which returns itself in DockerReference.
type refImageReferenceMock struct {
	mocks.ForbiddenImageReference
	ref reference.Named
}

func (ref refImageReferenceMock) Transport() types.ImageTransport {
	return mocks.NameImageTransport("== Transport mock")
}

func (ref refImageReferenceMock) StringWithinTransport() string {
	// We use this in error messages, so sadly we must return something. But right now we do so only when DockerReference is nil, so restrict to that.
	if ref.ref == nil {
		return "== StringWithinTransport for an image with no Docker support"
	}
	panic("unexpected call to a mock function")
}

func (ref refImageReferenceMock) DockerReference() reference.Named {
	return ref.ref
}

type prmSymmetricTableTest struct {
	refA, refB string
	result     bool
}

// Test cases for exact reference match. The behavior is supposed to be symmetric.
var prmExactMatchTestTable = []prmSymmetricTableTest{
	// Success, simple matches
	{"busybox:latest", "busybox:latest", true},
	{fullRHELRef, fullRHELRef, true},
	{"busybox" + digestSuffix, "busybox" + digestSuffix, true}, // NOTE: This is not documented; signing digests is not recommended at this time.
	// Non-canonical reference format is canonicalized
	{"library/busybox:latest", "busybox:latest", true},
	{"docker.io/library/busybox:latest", "busybox:latest", true},
	{"library/busybox" + digestSuffix, "busybox" + digestSuffix, true},
	// Mismatch
	{"busybox:latest", "busybox:notlatest", false},
	{"busybox:latest", "notbusybox:latest", false},
	{"busybox:latest", "hostname/library/busybox:notlatest", false},
	{"hostname/library/busybox:latest", "busybox:notlatest", false},
	{"busybox:latest", fullRHELRef, false},
	{"busybox" + digestSuffix, "notbusybox" + digestSuffix, false},
	{"busybox:latest", "busybox" + digestSuffix, false},
	{"busybox" + digestSuffix, "busybox" + digestSuffixOther, false},
	// NameOnly references
	{"busybox", "busybox:latest", false},
	{"busybox", "busybox" + digestSuffix, false},
	{"busybox", "busybox", false},
	// References with both tags and digests: We match them exactly (requiring BOTH to match)
	// NOTE: Again, this is not documented behavior; the recommendation is to sign tags, not digests, and then tag-and-digest references won’t match the signed identity.
	{"busybox:latest" + digestSuffix, "busybox:latest" + digestSuffix, true},
	{"busybox:latest" + digestSuffix, "busybox:latest" + digestSuffixOther, false},
	{"busybox:latest" + digestSuffix, "busybox:notlatest" + digestSuffix, false},
	{"busybox:latest" + digestSuffix, "busybox" + digestSuffix, false},
	{"busybox:latest" + digestSuffix, "busybox:latest", false},
	// Invalid format
	{"UPPERCASE_IS_INVALID_IN_DOCKER_REFERENCES", "busybox:latest", false},
	{"", "UPPERCASE_IS_INVALID_IN_DOCKER_REFERENCES", false},
	// Even if they are exactly equal, invalid values are rejected.
	{"INVALID", "INVALID", false},
}

// Test cases for repository-only reference match. The behavior is supposed to be symmetric.
var prmRepositoryMatchTestTable = []prmSymmetricTableTest{
	// Success, simple matches
	{"busybox:latest", "busybox:latest", true},
	{fullRHELRef, fullRHELRef, true},
	{"busybox" + digestSuffix, "busybox" + digestSuffix, true}, // NOTE: This is not documented; signing digests is not recommended at this time.
	// Non-canonical reference format is canonicalized
	{"library/busybox:latest", "busybox:latest", true},
	{"docker.io/library/busybox:latest", "busybox:latest", true},
	{"library/busybox" + digestSuffix, "busybox" + digestSuffix, true},
	// The same as above, but with mismatching tags
	{"busybox:latest", "busybox:notlatest", true},
	{fullRHELRef + "tagsuffix", fullRHELRef, true},
	{"library/busybox:latest", "busybox:notlatest", true},
	{"busybox:latest", "library/busybox:notlatest", true},
	{"docker.io/library/busybox:notlatest", "busybox:latest", true},
	{"busybox:notlatest", "docker.io/library/busybox:latest", true},
	{"busybox:latest", "busybox" + digestSuffix, true},
	{"busybox" + digestSuffix, "busybox" + digestSuffixOther, true}, // Even this is accepted here. (This could more reasonably happen with two different digest algorithms.)
	// The same as above, but with defaulted tags (which can happen with /usr/bin/cosign)
	{"busybox", "busybox:notlatest", true},
	{fullRHELRef, untaggedRHELRef, true},
	{"busybox", "busybox" + digestSuffix, true},
	{"library/busybox", "busybox", true},
	{"docker.io/library/busybox", "busybox", true},
	// Mismatch
	{"busybox:latest", "notbusybox:latest", false},
	{"hostname/library/busybox:latest", "busybox:notlatest", false},
	{"busybox:latest", fullRHELRef, false},
	{"busybox" + digestSuffix, "notbusybox" + digestSuffix, false},
	// References with both tags and digests: We ignore both anyway.
	{"busybox:latest" + digestSuffix, "busybox:latest" + digestSuffix, true},
	{"busybox:latest" + digestSuffix, "busybox:latest" + digestSuffixOther, true},
	{"busybox:latest" + digestSuffix, "busybox:notlatest" + digestSuffix, true},
	{"busybox:latest" + digestSuffix, "busybox" + digestSuffix, true},
	{"busybox:latest" + digestSuffix, "busybox:latest", true},
	// Invalid format
	{"UPPERCASE_IS_INVALID_IN_DOCKER_REFERENCES", "busybox:latest", false},
	{"", "UPPERCASE_IS_INVALID_IN_DOCKER_REFERENCES", false},
	// Even if they are exactly equal, invalid values are rejected.
	{"INVALID", "INVALID", false},
}

// Test cases for matchRepoDigestOrExact
var matchRepoDigestOrExactTestTable = []struct {
	imageRef, sigRef string
	result           bool
}{
	// Tag mismatch
	{"busybox:latest", "busybox:notlatest", false},
	{fullRHELRef + "tagsuffix", fullRHELRef, false},
	{"library/busybox:latest", "busybox:notlatest", false},
	{"busybox:latest", "library/busybox:notlatest", false},
	{"docker.io/library/busybox:notlatest", "busybox:latest", false},
	{"busybox:notlatest", "docker.io/library/busybox:latest", false},
	// NameOnly references
	{"busybox", "busybox:latest", false},
	{"busybox:latest", "busybox", false},
	{"busybox", "busybox" + digestSuffix, false},
	{"busybox" + digestSuffix, "busybox", false},
	{fullRHELRef, untaggedRHELRef, false},
	{"busybox", "busybox", false},
	// Tag references only accept signatures with matching tags.
	{"busybox:latest", "busybox" + digestSuffix, false},
	// Digest references accept any signature with matching repository.
	{"busybox" + digestSuffix, "busybox:latest", true},
	{"busybox" + digestSuffix, "busybox" + digestSuffixOther, true}, // Even this is accepted here. (This could more reasonably happen with two different digest algorithms.)
	// References with both tags and digests: We match them exactly (requiring BOTH to match).
	{"busybox:latest" + digestSuffix, "busybox:latest", false},
	{"busybox:latest" + digestSuffix, "busybox:notlatest", false},
	{"busybox:latest", "busybox:latest" + digestSuffix, false},
	{"busybox:latest" + digestSuffix, "busybox:latest" + digestSuffixOther, false},
	{"busybox:latest" + digestSuffix, "busybox:notlatest" + digestSuffixOther, false},
}

func testImageAndSig(t *testing.T, prm PolicyReferenceMatch, imageRef, sigRef string, result bool) {
	// This assumes that all ways to obtain a reference.Named perform equivalent validation,
	// and therefore values refused by reference.ParseNormalizedNamed can not happen in practice.
	parsedImageRef, err := reference.ParseNormalizedNamed(imageRef)
	require.NoError(t, err)
	res := prm.matchesDockerReference(refImageMock{ref: parsedImageRef}, sigRef)
	assert.Equal(t, result, res, fmt.Sprintf("%s vs. %s", imageRef, sigRef))
}

// testPossiblyInvalidImageAndSig is a variant of testImageAndSig
// that does not fail if the imageRef is invalid (which should never happen in practice,
// but makes testing of symmetrical properties using shared tables easier)
func testPossiblyInvalidImageAndSig(t *testing.T, prm PolicyReferenceMatch, imageRef, sigRef string, result bool) {
	// This assumes that all ways to obtain a reference.Named perform equivalent validation,
	// and therefore values refused by reference.ParseNormalizedNamed can not happen in practice.
	_, err := reference.ParseNormalizedNamed(imageRef)
	if err != nil {
		return
	}
	testImageAndSig(t, prm, imageRef, sigRef, result)
}

func TestMatchRepoDigestOrExactReferenceValues(t *testing.T) {
	// prmMatchRepoDigestOrExact is a middle ground between prmMatchExact and prmMatchRepository:
	// It accepts anything prmMatchExact accepts,…
	for _, test := range prmExactMatchTestTable {
		if test.result == true {
			refA, errA := reference.ParseNormalizedNamed(test.refA)
			refB, errB := reference.ParseNormalizedNamed(test.refB)
			if errA == nil && errB == nil {
				res1 := matchRepoDigestOrExactReferenceValues(refA, refB)
				assert.Equal(t, test.result, res1)
				res2 := matchRepoDigestOrExactReferenceValues(refB, refA)
				assert.Equal(t, test.result, res2)
			}
		}
	}
	// … and it rejects everything prmMatchRepository rejects.
	for _, test := range prmRepositoryMatchTestTable {
		if test.result == false {
			refA, errA := reference.ParseNormalizedNamed(test.refA)
			refB, errB := reference.ParseNormalizedNamed(test.refB)
			if errA == nil && errB == nil {
				res1 := matchRepoDigestOrExactReferenceValues(refA, refB)
				assert.Equal(t, test.result, res1)
				res2 := matchRepoDigestOrExactReferenceValues(refB, refA)
				assert.Equal(t, test.result, res2)
			}
		}
	}

	// The other cases, possibly asymmetrical:
	for _, test := range matchRepoDigestOrExactTestTable {
		imageRef, err := reference.ParseNormalizedNamed(test.imageRef)
		require.NoError(t, err)
		sigRef, err := reference.ParseNormalizedNamed(test.sigRef)
		require.NoError(t, err)
		res := matchRepoDigestOrExactReferenceValues(imageRef, sigRef)
		assert.Equal(t, test.result, res)
	}
}

func TestPRMMatchExactMatchesDockerReference(t *testing.T) {
	prm := NewPRMMatchExact()
	for _, test := range prmExactMatchTestTable {
		testPossiblyInvalidImageAndSig(t, prm, test.refA, test.refB, test.result)
		testPossiblyInvalidImageAndSig(t, prm, test.refB, test.refA, test.result)
	}
	// Even if they are signed with an empty string as a reference, unidentified images are rejected.
	res := prm.matchesDockerReference(refImageMock{ref: nil}, "")
	assert.False(t, res, `unidentified vs. ""`)
}

func TestPRMMatchRepoDigestOrExactMatchesDockerReference(t *testing.T) {
	prm := NewPRMMatchRepoDigestOrExact()

	// prmMatchRepoDigestOrExact is a middle ground between prmMatchExact and prmMatchRepository:
	// It accepts anything prmMatchExact accepts,…
	for _, test := range prmExactMatchTestTable {
		if test.result == true {
			testPossiblyInvalidImageAndSig(t, prm, test.refA, test.refB, test.result)
			testPossiblyInvalidImageAndSig(t, prm, test.refB, test.refA, test.result)
		}
	}
	// … and it rejects everything prmMatchRepository rejects.
	for _, test := range prmRepositoryMatchTestTable {
		if test.result == false {
			testPossiblyInvalidImageAndSig(t, prm, test.refA, test.refB, test.result)
			testPossiblyInvalidImageAndSig(t, prm, test.refB, test.refA, test.result)
		}
	}

	// The other cases, possibly asymmetrical:
	for _, test := range matchRepoDigestOrExactTestTable {
		testImageAndSig(t, prm, test.imageRef, test.sigRef, test.result)
	}
}

func TestPRMMatchRepositoryMatchesDockerReference(t *testing.T) {
	prm := NewPRMMatchRepository()
	for _, test := range prmRepositoryMatchTestTable {
		testPossiblyInvalidImageAndSig(t, prm, test.refA, test.refB, test.result)
		testPossiblyInvalidImageAndSig(t, prm, test.refB, test.refA, test.result)
	}
	// Even if they are signed with an empty string as a reference, unidentified images are rejected.
	res := prm.matchesDockerReference(refImageMock{ref: nil}, "")
	assert.False(t, res, `unidentified vs. ""`)
}

func TestParseDockerReferences(t *testing.T) {
	const (
		ok1  = "busybox"
		ok2  = fullRHELRef
		bad1 = "UPPERCASE_IS_INVALID_IN_DOCKER_REFERENCES"
		bad2 = ""
	)

	// Success
	r1, r2, err := parseDockerReferences(ok1, ok2)
	require.NoError(t, err)
	assert.Equal(t, ok1, reference.FamiliarString(r1))
	assert.Equal(t, ok2, reference.FamiliarString(r2))

	// Failures
	for _, refs := range [][]string{
		{bad1, ok2},
		{ok1, bad2},
		{bad1, bad2},
	} {
		_, _, err := parseDockerReferences(refs[0], refs[1])
		assert.Error(t, err)
	}
}

func testExactPRMAndSig(t *testing.T, prmFactory func(string) PolicyReferenceMatch, imageRef, sigRef string, result bool) {
	prm := prmFactory(imageRef)
	res := prm.matchesDockerReference(mocks.ForbiddenUnparsedImage{}, sigRef)
	assert.Equal(t, result, res, fmt.Sprintf("%s vs. %s", imageRef, sigRef))
}

func prmExactReferenceFactory(ref string) PolicyReferenceMatch {
	// Do not use NewPRMExactReference, we want to also test the case with an invalid DockerReference,
	// even though NewPRMExactReference should never let it happen.
	return &prmExactReference{DockerReference: ref}
}

func TestPRMExactReferenceMatchesDockerReference(t *testing.T) {
	for _, test := range prmExactMatchTestTable {
		testExactPRMAndSig(t, prmExactReferenceFactory, test.refA, test.refB, test.result)
		testExactPRMAndSig(t, prmExactReferenceFactory, test.refB, test.refA, test.result)
	}
}

func prmExactRepositoryFactory(ref string) PolicyReferenceMatch {
	// Do not use NewPRMExactRepository, we want to also test the case with an invalid DockerReference,
	// even though NewPRMExactRepository should never let it happen.
	return &prmExactRepository{DockerRepository: ref}
}

func TestPRMExactRepositoryMatchesDockerReference(t *testing.T) {
	for _, test := range prmRepositoryMatchTestTable {
		testExactPRMAndSig(t, prmExactRepositoryFactory, test.refA, test.refB, test.result)
		testExactPRMAndSig(t, prmExactRepositoryFactory, test.refB, test.refA, test.result)
	}
}

func TestPRMRemapIdentityRefMatchesPrefix(t *testing.T) {
	for _, c := range []struct {
		ref, prefix string
		expected    bool
	}{
		// Prefix is a reference.Domain() value
		{"docker.io/image", "docker.io", true},
		{"docker.io/image", "example.com", false},
		{"example.com:5000/image", "example.com:5000", true},
		{"example.com:50000/image", "example.com:5000", false},
		{"example.com:5000/image", "example.com", false},
		{"example.com/foo", "example.com", true},
		{"example.com/foo/bar", "example.com", true},
		{"example.com/foo/bar:baz", "example.com", true},
		{"example.com/foo/bar" + digestSuffix, "example.com", true},
		// Prefix is a reference.Named.Name() value or a repo namespace
		{"docker.io/ns/image", "docker.io/library", false},
		{"example.com/library", "docker.io/library", false},
		{"docker.io/libraryy/image", "docker.io/library", false},
		{"docker.io/library/busybox", "docker.io/library", true},
		{"example.com/ns/image", "example.com/ns", true},
		{"example.com/ns2/image", "example.com/ns", false},
		{"example.com/n2/image", "example.com/ns", false},
		{"example.com", "example.com/library/busybox", false},
		{"example.com:5000/ns/image", "example.com/ns", false},
		{"example.com/ns/image", "example.com:5000/ns", false},
		{"docker.io/library/busybox", "docker.io/library/busybox", true},
		{"example.com/library/busybox", "docker.io/library/busybox", false},
		{"docker.io/library/busybox2", "docker.io/library/busybox", false},
		{"example.com/ns/image", "example.com/ns/image", true},
		{"example.com/ns/imag2", "example.com/ns/image", false},
		{"example.com/ns/imagee", "example.com/ns/image", false},
		{"example.com:5000/ns/image", "example.com/ns/image", false},
		{"example.com/ns/image", "example.com:5000/ns/image", false},
		{"example.com/ns/image:tag", "example.com/ns/image", true},
		{"example.com/ns/image" + digestSuffix, "example.com/ns/image", true},
		{"example.com/ns/image:tag" + digestSuffix, "example.com/ns/image", true},
	} {
		prm, err := newPRMRemapIdentity(c.prefix, "docker.io/library/signed-prefix")
		require.NoError(t, err, c.prefix)
		ref, err := reference.ParseNormalizedNamed(c.ref)
		require.NoError(t, err, c.ref)
		res := prm.refMatchesPrefix(ref)
		assert.Equal(t, c.expected, res, fmt.Sprintf("%s vs. %s", c.ref, c.prefix))
	}
}

func TestPRMRemapIdentityRemapReferencePrefix(t *testing.T) {
	for _, c := range []struct{ prefix, signedPrefix, ref, expected string }{
		// Match sanity checking, primarily tested in TestPRMRefMatchesPrefix
		{"mirror.example", "vendor.example", "mirror.example/ns/image:tag", "vendor.example/ns/image:tag"},
		{"mirror.example", "vendor.example", "different.com/ns/image:tag", "different.com/ns/image:tag"},
		{"mirror.example/ns", "vendor.example/vendor-ns", "mirror.example/different-ns/image:tag", "mirror.example/different-ns/image:tag"},
		{"docker.io", "not-docker-signed.example/ns", "busybox", "not-docker-signed.example/ns/library/busybox"},
		// Rewrites work as expected
		{"mirror.example", "vendor.example", "mirror.example/ns/image:tag", "vendor.example/ns/image:tag"},
		{"example.com/mirror", "example.com/vendor", "example.com/mirror/image:tag", "example.com/vendor/image:tag"},
		{"example.com/ns/mirror", "example.com/ns/vendor", "example.com/ns/mirror:tag", "example.com/ns/vendor:tag"},
		{"mirror.example", "vendor.example", "prefixmirror.example/ns/image:tag", "prefixmirror.example/ns/image:tag"},
		{"docker.io", "not-docker-signed.example", "busybox", "not-docker-signed.example/library/busybox"},
		{"docker.io/library", "not-docker-signed.example/ns", "busybox", "not-docker-signed.example/ns/busybox"},
		{"docker.io/library/busybox", "not-docker-signed.example/ns/notbusybox", "busybox", "not-docker-signed.example/ns/notbusybox"},
		// On match, tag/digest is preserved
		{"mirror.example", "vendor.example", "mirror.example/image", "vendor.example/image"}, // This one should not actually happen, testing for completeness
		{"mirror.example", "vendor.example", "mirror.example/image:tag", "vendor.example/image:tag"},
		{"mirror.example", "vendor.example", "mirror.example/image" + digestSuffix, "vendor.example/image" + digestSuffix},
		{"mirror.example", "vendor.example", "mirror.example/image:tag" + digestSuffix, "vendor.example/image:tag" + digestSuffix},
		// Rewrite creating an invalid reference
		{"mirror.example/ns/image", "vendor.example:5000", "mirror.example/ns/image:tag", ""},
		// Rewrite creating a valid reference string in short format, which would imply a docker.io prefix and is rejected
		{"mirror.example/ns/image", "vendor.example:5000", "mirror.example/ns/image" + digestSuffix, ""}, // vendor.example:5000@digest
		{"mirror.example/ns/image", "notlocalhost", "mirror.example/ns/image:tag", ""},                   // notlocalhost:tag
	} {
		testName := fmt.Sprintf("%#v", c)
		prm, err := newPRMRemapIdentity(c.prefix, c.signedPrefix)
		require.NoError(t, err, testName)
		ref, err := reference.ParseNormalizedNamed(c.ref)
		require.NoError(t, err, testName)
		res, err := prm.remapReferencePrefix(ref)
		if c.expected == "" {
			assert.Error(t, err, testName)
		} else {
			require.NoError(t, err, testName)
			assert.Equal(t, c.expected, res.String(), testName)
		}
	}
}

// modifiedString returns some string that is different from the input,
// consistent across calls with the same input;
// in particular it just replaces the first letter.
func modifiedString(t *testing.T, input string) string {
	c := input[0]
	switch {
	case c >= 'a' && c <= 'y':
		c++
	case c == 'z':
		c = 'a'
	default:
		require.Fail(t, "unimplemented leading character '%c'", c)
	}
	return string(c) + input[1:]
}

// prmRemapIdentityMRDOETestCase is a helper for TestPRMRemapIdentityMatchesDockerReference,
// verifying that the behavior is consistent with prmMatchRepoDigestOrExact,
// while still smoke-testing the rewriting behavior.
// The test succeeds if imageRefString is invalid and ignoreInvalidImageRef.
func prmRemapIdentityMRDOETestCase(t *testing.T, ignoreInvalidImageRef bool, imageRef, sigRef string, result bool) {
	parsedImageRef, err := reference.ParseNormalizedNamed(imageRef)
	if ignoreInvalidImageRef && err != nil {
		return
	}
	require.NoError(t, err)

	// No rewriting happens.
	prm, err := NewPRMRemapIdentity("never-causes-a-rewrite.example", "never-causes-a-rewrite.example")
	require.NoError(t, err)
	testImageAndSig(t, prm, imageRef, sigRef, result)

	// Rewrite imageRef
	domain := reference.Domain(parsedImageRef)
	prm, err = NewPRMRemapIdentity(modifiedString(t, domain), domain)
	require.NoError(t, err)
	modifiedImageRef, err := reference.ParseNormalizedNamed(modifiedString(t, parsedImageRef.String()))
	require.NoError(t, err)
	testImageAndSig(t, prm, modifiedImageRef.String(), sigRef, result)
}

func TestPRMRemapIdentityMatchesDockerReference(t *testing.T) {
	// Basic sanity checks. More detailed testing is done in TestPRMRemapIdentityRemapReferencePrefix
	// and TestMatchRepoDigestOrExactReferenceValues.
	for _, c := range []struct {
		prefix, signedPrefix, imageRef, sigRef string
		result                                 bool
	}{
		// No match rewriting
		{"does-not-match.com", "does-not-match.rewritten", "busybox:latest", "busybox:latest", true},
		{"does-not-match.com", "does-not-match.rewritten", "busybox:latest", "notbusybox:latest", false},
		// Match rewriting non-docker
		{"mirror.example", "public.com", "mirror.example/busybox:1", "public.com/busybox:1", true},
		{"mirror.example", "public.com", "mirror.example/busybox:1", "public.com/busybox:not1", false},
		// Rewriting to docker.io
		{"mirror.example", "docker.io/library", "mirror.example/busybox:latest", "busybox:latest", true},
		{"mirror.example", "docker.io/library", "mirror.example/alpine:latest", "busybox:latest", false},
		// Rewriting from docker.io
		{"docker.io/library", "original.com", "copied:latest", "original.com/copied:latest", true},
		{"docker.io/library", "original.com", "copied:latest", "original.com/ns/copied:latest", false},
		// Invalid object: prefix is not a host name
		{"busybox", "example.com/busybox", "busybox:latest", "example.com/busybox:latest", false},
		// Invalid object: signedPrefix is not a host name
		{"docker.io/library/busybox", "busybox", "docker.io/library/busybox:latest", "busybox:latest", false},
		// Invalid object: invalid prefix
		{"UPPERCASE", "example.com", "example.com/foo:latest", "example.com/foo:latest", true}, // Happens to work, not an API promise
		{"example.com", "UPPERCASE", "example.com/foo:latest", "UPPERCASE/foo:latest", false},
	} {
		// Do not use NewPRMRemapIdentity, we want to also test the cases with invalid values,
		// even though NewPRMExactReference should never let it happen.
		prm := &prmRemapIdentity{Prefix: c.prefix, SignedPrefix: c.signedPrefix}
		testImageAndSig(t, prm, c.imageRef, c.sigRef, c.result)
	}
	// Even if they are signed with an empty string as a reference, unidentified images are rejected.
	prm, err := NewPRMRemapIdentity("docker.io", "docker.io")
	require.NoError(t, err)
	res := prm.matchesDockerReference(refImageMock{ref: nil}, "")
	assert.False(t, res, `unidentified vs. ""`)

	// Verify that the behavior is otherwise the same as for prmMatchRepoDigestOrExact:
	// prmMatchRepoDigestOrExact is a middle ground between prmMatchExact and prmMatchRepository:
	// It accepts anything prmMatchExact accepts,…
	for _, test := range prmExactMatchTestTable {
		if test.result == true {
			prmRemapIdentityMRDOETestCase(t, true, test.refA, test.refB, test.result)
			prmRemapIdentityMRDOETestCase(t, true, test.refB, test.refA, test.result)
		}
	}
	// … and it rejects everything prmMatchRepository rejects.
	for _, test := range prmRepositoryMatchTestTable {
		if test.result == false {
			prmRemapIdentityMRDOETestCase(t, true, test.refA, test.refB, test.result)
			prmRemapIdentityMRDOETestCase(t, true, test.refB, test.refA, test.result)
		}
	}

	// The other cases, possibly asymmetrical:
	for _, test := range matchRepoDigestOrExactTestTable {
		prmRemapIdentityMRDOETestCase(t, false, test.imageRef, test.sigRef, test.result)
	}
}

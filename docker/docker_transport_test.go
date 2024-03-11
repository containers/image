package docker

import (
	"context"
	"strings"
	"testing"

	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	sha256digestHex         = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	sha256digest            = "@sha256:" + sha256digestHex
	unknownDigestSuffixTest = "@@unknown-digest@@"
)

func TestTransportName(t *testing.T) {
	assert.Equal(t, "docker", Transport.Name())
}

func TestTransportParseReference(t *testing.T) {
	testParseReference(t, Transport.ParseReference)
}

func TestTransportValidatePolicyConfigurationScope(t *testing.T) {
	for _, scope := range []string{
		"docker.io/library/busybox" + sha256digest,
		"docker.io/library/busybox:notlatest",
		"docker.io/library/busybox",
		"docker.io/library",
		"docker.io",
		"*.io",
	} {
		err := Transport.ValidatePolicyConfigurationScope(scope)
		assert.NoError(t, err, scope)
	}
}

func TestParseReference(t *testing.T) {
	testParseReference(t, ParseReference)
}

// testParseReference is a test shared for Transport.ParseReference and ParseReference.
func testParseReference(t *testing.T, fn func(string) (types.ImageReference, error)) {
	for _, c := range []struct {
		input, expected       string
		expectedUnknownDigest bool
	}{
		{"busybox", "", false}, // Missing // prefix
		{"//busybox:notlatest", "docker.io/library/busybox:notlatest", false},           // Explicit tag
		{"//busybox" + sha256digest, "docker.io/library/busybox" + sha256digest, false}, // Explicit digest
		{"//busybox", "docker.io/library/busybox:latest", false},                        // Default tag
		// A github.com/distribution/reference value can have a tag and a digest at the same time!
		// The docker/distribution API does not really support that (we can’t ask for an image with a specific
		// tag and digest), so fail.  This MAY be accepted in the future.
		{"//busybox:latest" + sha256digest, "", false},                                         // Both tag and digest
		{"//docker.io/library/busybox:latest", "docker.io/library/busybox:latest", false},      // All implied values explicitly specified
		{"//UPPERCASEISINVALID", "", false},                                                    // Invalid input
		{"//busybox" + unknownDigestSuffixTest, "docker.io/library/busybox", true},             // UnknownDigest suffix
		{"//example.com/ns/busybox" + unknownDigestSuffixTest, "example.com/ns/busybox", true}, // UnknownDigest with registry/repo
		{"//example.com/ns/busybox:tag1" + unknownDigestSuffixTest, "", false},                 // UnknownDigest with tag should fail
		{"//example.com/ns/busybox" + sha256digest + unknownDigestSuffixTest, "", false},       // UnknownDigest with digest should fail
	} {
		ref, err := fn(c.input)
		if c.expected == "" {
			assert.Error(t, err, c.input)
		} else {
			require.NoError(t, err, c.input)
			dockerRef, ok := ref.(dockerReference)
			require.True(t, ok, c.input)
			assert.Equal(t, c.expected, dockerRef.ref.String(), c.input)
			assert.Equal(t, c.expectedUnknownDigest, dockerRef.isUnknownDigest)
		}
	}
}

// A common list of reference formats to test for the various ImageReference methods.
var validReferenceTestCases = []struct {
	input, dockerRef, stringWithinTransport string
	expectedUnknownDigest                   bool
}{
	{"busybox:notlatest", "docker.io/library/busybox:notlatest", "//busybox:notlatest", false},                                                 // Explicit tag
	{"busybox" + sha256digest, "docker.io/library/busybox" + sha256digest, "//busybox" + sha256digest, false},                                  // Explicit digest
	{"docker.io/library/busybox:latest", "docker.io/library/busybox:latest", "//busybox:latest", false},                                        // All implied values explicitly specified
	{"example.com/ns/foo:bar", "example.com/ns/foo:bar", "//example.com/ns/foo:bar", false},                                                    // All values explicitly specified
	{"example.com/ns/busybox" + unknownDigestSuffixTest, "example.com/ns/busybox", "//example.com/ns/busybox" + unknownDigestSuffixTest, true}, // UnknownDigest Suffix full name
	{"busybox" + unknownDigestSuffixTest, "docker.io/library/busybox", "//busybox" + unknownDigestSuffixTest, true},                            // UnknownDigest short name
}

func TestNewReference(t *testing.T) {
	for _, c := range validReferenceTestCases {
		if strings.HasSuffix(c.input, unknownDigestSuffixTest) {
			continue
		}
		parsed, err := reference.ParseNormalizedNamed(c.input)
		require.NoError(t, err)
		ref, err := NewReference(parsed)
		require.NoError(t, err, c.input)
		dockerRef, ok := ref.(dockerReference)
		require.True(t, ok, c.input)
		assert.Equal(t, c.dockerRef, dockerRef.ref.String(), c.input)
		assert.Equal(t, false, dockerRef.isUnknownDigest)
	}

	// Neither a tag nor digest
	parsed, err := reference.ParseNormalizedNamed("busybox")
	require.NoError(t, err)
	_, err = NewReference(parsed)
	assert.Error(t, err)

	// A github.com/distribution/reference value can have a tag and a digest at the same time!
	parsed, err = reference.ParseNormalizedNamed("busybox:notlatest" + sha256digest)
	require.NoError(t, err)
	_, ok := parsed.(reference.Canonical)
	require.True(t, ok)
	_, ok = parsed.(reference.NamedTagged)
	require.True(t, ok)
	_, err = NewReference(parsed)
	assert.Error(t, err)
}

func TestNewReferenceUnknownDigest(t *testing.T) {
	// References with tags and digests should be rejected
	for _, c := range validReferenceTestCases {
		in, ok := strings.CutSuffix(c.input, unknownDigestSuffixTest)
		if !ok {
			parsed, err := reference.ParseNormalizedNamed(c.input)
			require.NoError(t, err)
			_, err = NewReferenceUnknownDigest(parsed)
			assert.Error(t, err)
			continue
		}
		parsed, err := reference.ParseNormalizedNamed(in)
		require.NoError(t, err)
		ref, err := NewReferenceUnknownDigest(parsed)
		require.NoError(t, err, c.input)
		dockerRef, ok := ref.(dockerReference)
		require.True(t, ok, c.input)
		assert.Equal(t, c.dockerRef, dockerRef.ref.String(), c.input)
		assert.Equal(t, true, dockerRef.isUnknownDigest)
	}
}

func TestReferenceTransport(t *testing.T) {
	ref, err := ParseReference("//busybox")
	require.NoError(t, err)
	assert.Equal(t, Transport, ref.Transport())
}

func TestReferenceStringWithinTransport(t *testing.T) {
	for _, c := range validReferenceTestCases {
		ref, err := ParseReference("//" + c.input)
		require.NoError(t, err, c.input)
		stringRef := ref.StringWithinTransport()
		assert.Equal(t, c.stringWithinTransport, stringRef, c.input)
		// Do one more round to verify that the output can be parsed, to an equal value.
		ref2, err := Transport.ParseReference(stringRef)
		require.NoError(t, err, c.input)
		stringRef2 := ref2.StringWithinTransport()
		assert.Equal(t, stringRef, stringRef2, c.input)
	}
}

func TestReferenceDockerReference(t *testing.T) {
	for _, c := range validReferenceTestCases {
		ref, err := ParseReference("//" + c.input)
		require.NoError(t, err, c.input)
		dockerRef := ref.DockerReference()
		require.NotNil(t, dockerRef, c.input)
		assert.Equal(t, c.dockerRef, dockerRef.String(), c.input)
	}
}

func TestReferencePolicyConfigurationIdentity(t *testing.T) {
	// Just a smoke test, the substance is tested in policyconfiguration.TestDockerReference.
	ref, err := ParseReference("//busybox")
	require.NoError(t, err)
	assert.Equal(t, "docker.io/library/busybox:latest", ref.PolicyConfigurationIdentity())

	ref, err = ParseReference("//busybox" + unknownDigestSuffixTest)
	require.NoError(t, err)
	assert.Equal(t, "docker.io/library/busybox", ref.PolicyConfigurationIdentity())
}

func TestReferencePolicyConfigurationNamespaces(t *testing.T) {
	// Just a smoke test, the substance is tested in policyconfiguration.TestDockerReference.
	ref, err := ParseReference("//busybox")
	require.NoError(t, err)
	assert.Equal(t, []string{
		"docker.io/library/busybox",
		"docker.io/library",
		"docker.io",
		"*.io",
	}, ref.PolicyConfigurationNamespaces())

	ref, err = ParseReference("//busybox" + unknownDigestSuffixTest)
	require.NoError(t, err)
	assert.Equal(t, []string{
		"docker.io/library",
		"docker.io",
		"*.io",
	}, ref.PolicyConfigurationNamespaces())
}

func TestReferenceNewImage(t *testing.T) {
	sysCtx := &types.SystemContext{
		RegistriesDirPath:        "/this/does/not/exist",
		DockerPerHostCertDirPath: "/this/does/not/exist",
		ArchitectureChoice:       "amd64",
		OSChoice:                 "linux",
	}
	ref, err := ParseReference("//quay.io/libpod/busybox")
	require.NoError(t, err)
	img, err := ref.NewImage(context.Background(), sysCtx)
	require.NoError(t, err)
	defer img.Close()

	// unknownDigest case should return error
	ref, err = ParseReference("//quay.io/libpod/busybox" + unknownDigestSuffixTest)
	require.NoError(t, err)
	_, err = ref.NewImage(context.Background(), sysCtx)
	assert.Error(t, err)
}

func TestReferenceNewImageSource(t *testing.T) {
	sysCtx := &types.SystemContext{
		RegistriesDirPath:        "/this/does/not/exist",
		DockerPerHostCertDirPath: "/this/does/not/exist",
	}
	ref, err := ParseReference("//quay.io/libpod/busybox")
	require.NoError(t, err)
	src, err := ref.NewImageSource(context.Background(), sysCtx)
	require.NoError(t, err)
	defer src.Close()

	// unknownDigest case should return error
	ref, err = ParseReference("//quay.io/libpod/busybox" + unknownDigestSuffixTest)
	require.NoError(t, err)
	_, err = ref.NewImageSource(context.Background(), sysCtx)
	assert.Error(t, err)
}

func TestReferenceNewImageDestination(t *testing.T) {
	ref, err := ParseReference("//quay.io/libpod/busybox")
	require.NoError(t, err)
	dest, err := ref.NewImageDestination(context.Background(),
		&types.SystemContext{RegistriesDirPath: "/this/does/not/exist", DockerPerHostCertDirPath: "/this/does/not/exist"})
	require.NoError(t, err)
	defer dest.Close()

	ref, err = ParseReference("//quay.io/libpod/busybox" + unknownDigestSuffixTest)
	require.NoError(t, err)
	dest2, err := ref.NewImageDestination(context.Background(),
		&types.SystemContext{RegistriesDirPath: "/this/does/not/exist", DockerPerHostCertDirPath: "/this/does/not/exist"})
	require.NoError(t, err)
	defer dest2.Close()
}

func TestReferenceTagOrDigest(t *testing.T) {
	for input, expected := range map[string]string{
		"//busybox:notlatest":      "notlatest",
		"//busybox" + sha256digest: "sha256:" + sha256digestHex,
	} {
		ref, err := ParseReference(input)
		require.NoError(t, err, input)
		dockerRef, ok := ref.(dockerReference)
		require.True(t, ok, input)
		tod, err := dockerRef.tagOrDigest()
		require.NoError(t, err, input)
		assert.Equal(t, expected, tod, input)
	}

	// Invalid input
	ref, err := reference.ParseNormalizedNamed("busybox")
	require.NoError(t, err)
	dockerRef := dockerReference{ref: ref}
	_, err = dockerRef.tagOrDigest()
	assert.Error(t, err)

	// Invalid input, unknownDigest case
	ref, err = reference.ParseNormalizedNamed("busybox")
	require.NoError(t, err)
	dockerRef = dockerReference{ref: ref, isUnknownDigest: true}
	_, err = dockerRef.tagOrDigest()
	assert.Error(t, err)
}

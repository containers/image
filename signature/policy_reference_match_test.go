package signature

import (
	"fmt"
	"testing"

	"github.com/containers/image/types"
	"github.com/docker/docker/reference"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	fullRHELRef     = "registry.access.redhat.com/rhel7/rhel:7.2.3"
	untaggedRHELRef = "registry.access.redhat.com/rhel7/rhel"
)

func TestParseImageAndDockerReference(t *testing.T) {
	const (
		ok1  = "busybox"
		ok2  = fullRHELRef
		bad1 = "UPPERCASE_IS_INVALID_IN_DOCKER_REFERENCES"
		bad2 = ""
	)
	// Success
	ref, err := reference.ParseNamed(ok1)
	require.NoError(t, err)
	r1, r2, err := parseImageAndDockerReference(refImageMock{ref}, ok2)
	require.NoError(t, err)
	assert.Equal(t, ok1, r1.String())
	assert.Equal(t, ok2, r2.String())

	// Unidentified images are rejected.
	_, _, err = parseImageAndDockerReference(refImageMock{nil}, ok2)
	require.Error(t, err)
	assert.IsType(t, PolicyRequirementError(""), err)

	// Failures
	for _, refs := range [][]string{
		{bad1, ok2},
		{ok1, bad2},
		{bad1, bad2},
	} {
		ref, err := reference.ParseNamed(refs[0])
		if err == nil {
			_, _, err := parseImageAndDockerReference(refImageMock{ref}, refs[1])
			assert.Error(t, err)
		}
	}
}

// refImageMock is a mock of types.Image which returns itself in Reference().DockerReference.
type refImageMock struct{ reference.Named }

func (ref refImageMock) Reference() types.ImageReference {
	return refImageReferenceMock{ref.Named}
}
func (ref refImageMock) Close() {
	panic("unexpected call to a mock function")
}
func (ref refImageMock) Manifest() ([]byte, string, error) {
	panic("unexpected call to a mock function")
}
func (ref refImageMock) ManifestMatchesDigest(expectedDigest string) (bool, error) {
	panic("unexpected call to a mock function")
}
func (ref refImageMock) Signatures() ([][]byte, error) {
	panic("unexpected call to a mock function")
}
func (ref refImageMock) ConfigDigest() (string, error) {
	panic("unexpected call to a mock function")
}
func (ref refImageMock) LayerDigests() ([]string, error) {
	panic("unexpected call to a mock function")
}
func (ref refImageMock) Inspect() (*types.ImageInspectInfo, error) {
	panic("unexpected call to a mock function")
}
func (ref refImageMock) GetRepositoryTags() ([]string, error) {
	panic("unexpected call to a mock function")
}

// refImageReferenceMock is a mock of types.ImageReference which returns itself in DockerReference.
type refImageReferenceMock struct{ reference.Named }

func (ref refImageReferenceMock) Transport() types.ImageTransport {
	// We use this in error messages, so sadly we must return something. But right now we do so only when DockerReference is nil, so restrict to that.
	if ref.Named == nil {
		return nameImageTransportMock("== Transport mock")
	}
	panic("unexpected call to a mock function")
}
func (ref refImageReferenceMock) StringWithinTransport() string {
	// We use this in error messages, so sadly we must return something. But right now we do so only when DockerReference is nil, so restrict to that.
	if ref.Named == nil {
		return "== StringWithinTransport for an image with no Docker support"
	}
	panic("unexpected call to a mock function")
}
func (ref refImageReferenceMock) DockerReference() reference.Named {
	return ref.Named
}
func (ref refImageReferenceMock) PolicyConfigurationIdentity() string {
	panic("unexpected call to a mock function")
}
func (ref refImageReferenceMock) PolicyConfigurationNamespaces() []string {
	panic("unexpected call to a mock function")
}
func (ref refImageReferenceMock) NewImage(ctx *types.SystemContext) (types.Image, error) {
	panic("unexpected call to a mock function")
}
func (ref refImageReferenceMock) NewImageSource(ctx *types.SystemContext, requestedManifestMIMETypes []string) (types.ImageSource, error) {
	panic("unexpected call to a mock function")
}
func (ref refImageReferenceMock) NewImageDestination(ctx *types.SystemContext) (types.ImageDestination, error) {
	panic("unexpected call to a mock function")
}
func (ref refImageReferenceMock) DeleteImage(ctx *types.SystemContext) error {
	panic("unexpected call to a mock function")
}

// nameImageTransportMock is a mock of types.ImageTransport which returns itself in Name.
type nameImageTransportMock string

func (name nameImageTransportMock) Name() string {
	return string(name)
}
func (name nameImageTransportMock) ParseReference(reference string) (types.ImageReference, error) {
	panic("unexpected call to a mock function")
}
func (name nameImageTransportMock) ValidatePolicyConfigurationScope(scope string) error {
	panic("unexpected call to a mock function")
}

type prmTableTest struct {
	imageRef, sigRef string
	result           bool
}

// Test cases for exact reference match
var prmExactMatchTestTable = []prmTableTest{
	// Success, simple matches
	{"busybox:latest", "busybox:latest", true},
	{fullRHELRef, fullRHELRef, true},
	// Non-canonical reference format is canonicalized
	{"library/busybox:latest", "busybox:latest", true},
	{"busybox:latest", "library/busybox:latest", true},
	{"docker.io/library/busybox:latest", "busybox:latest", true},
	{"busybox:latest", "docker.io/library/busybox:latest", true},
	// Mismatch
	{"busybox:latest", "busybox:notlatest", false},
	{"busybox:latest", "notbusybox:latest", false},
	{"busybox:latest", "hostname/library/busybox:notlatest", false},
	{"hostname/library/busybox:latest", "busybox:notlatest", false},
	{"busybox:latest", fullRHELRef, false},
	// Missing tags
	{"busybox", "busybox:latest", false},
	{"busybox:latest", "busybox", false},
	{"busybox", "busybox", false},
	// Invalid format
	{"UPPERCASE_IS_INVALID_IN_DOCKER_REFERENCES", "busybox:latest", false},
	{"busybox:latest", "UPPERCASE_IS_INVALID_IN_DOCKER_REFERENCES", false},
	{"", "UPPERCASE_IS_INVALID_IN_DOCKER_REFERENCES", false},
	// Even if they are exactly equal, invalid values are rejected.
	{"INVALID", "INVALID", false},
}

// Test cases for repository-only reference match
var prmRepositoryMatchTestTable = []prmTableTest{
	// Success, simple matches
	{"busybox:latest", "busybox:latest", true},
	{fullRHELRef, fullRHELRef, true},
	// Non-canonical reference format is canonicalized
	{"library/busybox:latest", "busybox:latest", true},
	{"busybox:latest", "library/busybox:latest", true},
	{"docker.io/library/busybox:latest", "busybox:latest", true},
	{"busybox:latest", "docker.io/library/busybox:latest", true},
	// The same as above, but with mismatching tags
	{"busybox:latest", "busybox:notlatest", true},
	{fullRHELRef + "tagsuffix", fullRHELRef, true},
	{"library/busybox:latest", "busybox:notlatest", true},
	{"busybox:latest", "library/busybox:notlatest", true},
	{"docker.io/library/busybox:notlatest", "busybox:latest", true},
	{"busybox:notlatest", "docker.io/library/busybox:latest", true},
	// The same as above, but with defaulted tags (should not actually happen)
	{"busybox", "busybox:notlatest", true},
	{fullRHELRef, untaggedRHELRef, true},
	{"library/busybox", "busybox", true},
	{"busybox", "library/busybox", true},
	{"docker.io/library/busybox", "busybox", true},
	{"busybox", "docker.io/library/busybox", true},
	// Mismatch
	{"busybox:latest", "notbusybox:latest", false},
	{"hostname/library/busybox:latest", "busybox:notlatest", false},
	{"busybox:latest", fullRHELRef, false},
	// Invalid format
	{"UPPERCASE_IS_INVALID_IN_DOCKER_REFERENCES", "busybox:latest", false},
	{"busybox:latest", "UPPERCASE_IS_INVALID_IN_DOCKER_REFERENCES", false},
	{"", "UPPERCASE_IS_INVALID_IN_DOCKER_REFERENCES", false},
	// Even if they are exactly equal, invalid values are rejected.
	{"INVALID", "INVALID", false},
}

func TestPRMMatchExactMatchesDockerReference(t *testing.T) {
	prm := NewPRMMatchExact()
	for _, test := range prmExactMatchTestTable {
		// This assumes that all ways to obtain a reference.Named perform equivalent validation,
		// and therefore values refused by reference.ParseNamed can not happen in practice.
		imageRef, err := reference.ParseNamed(test.imageRef)
		if err != nil {
			continue
		}
		res := prm.matchesDockerReference(refImageMock{imageRef}, test.sigRef)
		assert.Equal(t, test.result, res, fmt.Sprintf("%s vs. %s", test.imageRef, test.sigRef))
	}
	// Even if they are signed with an empty string as a reference, unidentified images are rejected.
	res := prm.matchesDockerReference(refImageMock{nil}, "")
	assert.False(t, res, `unidentified vs. ""`)
}

func TestPRMMatchRepositoryMatchesDockerReference(t *testing.T) {
	prm := NewPRMMatchRepository()
	for _, test := range prmRepositoryMatchTestTable {
		// This assumes that all ways to obtain a reference.Named perform equivalent validation,
		// and therefore values refused by reference.ParseNamed can not happen in practice.
		imageRef, err := reference.ParseNamed(test.imageRef)
		if err != nil {
			continue
		}
		res := prm.matchesDockerReference(refImageMock{imageRef}, test.sigRef)
		assert.Equal(t, test.result, res, fmt.Sprintf("%s vs. %s", test.imageRef, test.sigRef))
	}
	// Even if they are signed with an empty string as a reference, unidentified images are rejected.
	res := prm.matchesDockerReference(refImageMock{nil}, "")
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
	assert.Equal(t, ok1, r1.String())
	assert.Equal(t, ok2, r2.String())

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

// forbiddenImageMock is a mock of types.Image which ensures Reference is not called
type forbiddenImageMock struct{}

func (ref forbiddenImageMock) Reference() types.ImageReference {
	panic("unexpected call to a mock function")
}
func (ref forbiddenImageMock) Close() {
	panic("unexpected call to a mock function")
}
func (ref forbiddenImageMock) Manifest() ([]byte, string, error) {
	panic("unexpected call to a mock function")
}
func (ref forbiddenImageMock) ManifestMatchesDigest(expectedDigest string) (bool, error) {
	panic("unexpected call to a mock function")
}
func (ref forbiddenImageMock) Signatures() ([][]byte, error) {
	panic("unexpected call to a mock function")
}
func (ref forbiddenImageMock) ConfigDigest() (string, error) {
	panic("unexpected call to a mock function")
}
func (ref forbiddenImageMock) LayerDigests() ([]string, error) {
	panic("unexpected call to a mock function")
}
func (ref forbiddenImageMock) Inspect() (*types.ImageInspectInfo, error) {
	panic("unexpected call to a mock function")
}
func (ref forbiddenImageMock) GetRepositoryTags() ([]string, error) {
	panic("unexpected call to a mock function")
}

func TestPRMExactReferenceMatchesDockerReference(t *testing.T) {
	for _, test := range prmExactMatchTestTable {
		// Do not use NewPRMExactReference, we want to also test the case with an invalid DockerReference,
		// even though NewPRMExactReference should never let it happen.
		prm := prmExactReference{DockerReference: test.imageRef}
		res := prm.matchesDockerReference(forbiddenImageMock{}, test.sigRef)
		assert.Equal(t, test.result, res, fmt.Sprintf("%s vs. %s", test.imageRef, test.sigRef))
	}
}

func TestPRMExactRepositoryMatchesDockerReference(t *testing.T) {
	for _, test := range prmRepositoryMatchTestTable {
		// Do not use NewPRMExactRepository, we want to also test the case with an invalid DockerReference,
		// even though NewPRMExactRepository should never let it happen.
		prm := prmExactRepository{DockerRepository: test.imageRef}
		res := prm.matchesDockerReference(forbiddenImageMock{}, test.sigRef)
		assert.Equal(t, test.result, res, fmt.Sprintf("%s vs. %s", test.imageRef, test.sigRef))
	}
}

package internal

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

type testDataSplitReference struct {
	ref   string
	dir   string
	image string
}

type testDataScopeValidation struct {
	scope      string
	errMessage string
}

func TestSplitReferenceIntoDirAndImageWindows(t *testing.T) {
	tests := []testDataSplitReference{
		{`C:\foo\bar:busybox:latest`, `C:\foo\bar`, "busybox:latest"},
		{`C:\foo\bar:busybox`, `C:\foo\bar`, "busybox"},
		{`C:\foo\bar`, `C:\foo\bar`, ""},
	}
	for _, test := range tests {
		dir, image := splitPathAndImageWindows(test.ref)
		assert.Equal(t, test.dir, dir, "Unexpected OCI directory")
		assert.Equal(t, test.image, image, "Unexpected image")
	}
}

func TestSplitReferenceIntoDirAndImageNonWindows(t *testing.T) {
	tests := []testDataSplitReference{
		{"/foo/bar:busybox:latest", "/foo/bar", "busybox:latest"},
		{"/foo/bar:busybox", "/foo/bar", "busybox"},
		{"/foo/bar", "/foo/bar", ""},
	}
	for _, test := range tests {
		dir, image := splitPathAndImageNonWindows(test.ref)
		assert.Equal(t, test.dir, dir, "Unexpected OCI directory")
		assert.Equal(t, test.image, image, "Unexpected image")
	}
}

func TestValidateScopeWindows(t *testing.T) {
	tests := []testDataScopeValidation{
		{`C:\foo`, ""},
		{`D:\`, ""},
		{"C:", "Invalid scope 'C:'. Must be an absolute path"},
		{"E", "Invalid scope 'E'. Must be an absolute path"},
		{"", "Invalid scope ''. Must be an absolute path"},
	}
	for _, test := range tests {
		err := validateScopeWindows(test.scope)
		if test.errMessage == "" {
			assert.NoError(t, err)
		} else {
			assert.EqualError(t, err, test.errMessage, fmt.Sprintf("No error for scope '%s'", test.scope))
		}
	}
}

func TestParseOCIReferenceName(t *testing.T) {
	image, idx, err := ParseOCIReferenceName("@0")
	assert.NoError(t, err)
	assert.Equal(t, image, "")
	assert.Equal(t, idx, 0)

	image, idx, err = ParseOCIReferenceName("notlatest@1")
	assert.NoError(t, err)
	assert.Equal(t, image, "notlatest@1")
	assert.Equal(t, idx, -1)

	_, _, err = ParseOCIReferenceName("@-5")
	assert.NotEmpty(t, err)

	_, _, err = ParseOCIReferenceName("@invalidIndex")
	assert.NotEmpty(t, err)
}

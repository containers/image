//go:build !containers_image_storage_stub
// +build !containers_image_storage_stub

package storage

import (
	"fmt"
	"strings"
	"testing"

	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/pkg/blobinfocache/memory"
	"github.com/containers/storage"
	"github.com/containers/storage/pkg/archive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewReference(t *testing.T) {
	newStore(t)
	st, ok := Transport.(*storageTransport)
	require.True(t, ok)
	// Success is tested throughout; test only the failure
	_, err := newReference(*st, nil, "")
	assert.Error(t, err)
	_, err = newReference(*st, nil, "ab")
	assert.Error(t, err)
	ref, err := reference.ParseNormalizedNamed("busybox")
	require.NoError(t, err)
	_, err = newReference(*st, ref, "")
	assert.Error(t, err)
}

func TestStorageReferenceTransport(t *testing.T) {
	newStore(t)
	ref, err := Transport.ParseReference("busybox")
	require.NoError(t, err)
	transport := ref.Transport()
	st, ok := transport.(*storageTransport)
	require.True(t, ok)
	assert.Equal(t, *(Transport.(*storageTransport)), *st)
}

// A common list of reference formats to test for the various ImageReference methods.
var validReferenceTestCases = []struct {
	input, dockerRef, canonical string
	namespaces                  []string
}{
	{
		"busybox", "docker.io/library/busybox:latest", "docker.io/library/busybox:latest",
		[]string{"docker.io/library/busybox", "docker.io/library", "docker.io"},
	},
	{
		"example.com/myns/ns2/busybox:notlatest", "example.com/myns/ns2/busybox:notlatest", "example.com/myns/ns2/busybox:notlatest",
		[]string{"example.com/myns/ns2/busybox", "example.com/myns/ns2", "example.com/myns", "example.com"},
	},
	{
		"@" + sha256digestHex, "", "@" + sha256digestHex,
		[]string{},
	},
	{
		"busybox@" + sha256digestHex, "docker.io/library/busybox:latest", "docker.io/library/busybox:latest@" + sha256digestHex,
		[]string{"docker.io/library/busybox:latest", "docker.io/library/busybox", "docker.io/library", "docker.io"},
	},
	{
		"busybox@sha256:" + sha256digestHex, "docker.io/library/busybox@sha256:" + sha256digestHex, "docker.io/library/busybox@sha256:" + sha256digestHex,
		[]string{"docker.io/library/busybox", "docker.io/library", "docker.io"},
	},
	{
		"busybox:notlatest@" + sha256digestHex, "docker.io/library/busybox:notlatest", "docker.io/library/busybox:notlatest@" + sha256digestHex,
		[]string{"docker.io/library/busybox:notlatest", "docker.io/library/busybox", "docker.io/library", "docker.io"},
	},
	{
		"busybox:notlatest@sha256:" + sha256digestHex, "docker.io/library/busybox:notlatest@sha256:" + sha256digestHex, "docker.io/library/busybox:notlatest@sha256:" + sha256digestHex,
		[]string{"docker.io/library/busybox:notlatest", "docker.io/library/busybox", "docker.io/library", "docker.io"},
	},
	{
		"busybox@" + sha256Digest2 + "@" + sha256digestHex, "docker.io/library/busybox@" + sha256Digest2, "docker.io/library/busybox@" + sha256Digest2 + "@" + sha256digestHex,
		[]string{"docker.io/library/busybox@" + sha256Digest2, "docker.io/library/busybox", "docker.io/library", "docker.io"},
	},
	{
		"busybox:notlatest@" + sha256Digest2 + "@" + sha256digestHex, "docker.io/library/busybox:notlatest@" + sha256Digest2, "docker.io/library/busybox:notlatest@" + sha256Digest2 + "@" + sha256digestHex,
		[]string{"docker.io/library/busybox:notlatest@" + sha256Digest2, "docker.io/library/busybox:notlatest", "docker.io/library/busybox", "docker.io/library", "docker.io"},
	},
}

func TestStorageReferenceDockerReference(t *testing.T) {
	newStore(t)
	for _, c := range validReferenceTestCases {
		ref, err := Transport.ParseReference(c.input)
		require.NoError(t, err, c.input)
		if c.dockerRef != "" {
			dr := ref.DockerReference()
			require.NotNil(t, dr, c.input)
			assert.Equal(t, c.dockerRef, dr.String(), c.input)
		} else {
			dr := ref.DockerReference()
			assert.Nil(t, dr, c.input)
		}
	}
}

// The […] part of references created for store
func storeSpecForStringWithinTransport(store storage.Store) string {
	optionsList := ""
	options := store.GraphOptions()
	if len(options) > 0 {
		optionsList = ":" + strings.Join(options, ",")
	}
	return fmt.Sprintf("[%s@%s+%s%s]", store.GraphDriverName(), store.GraphRoot(), store.RunRoot(), optionsList)
}

func TestStorageReferenceStringWithinTransport(t *testing.T) {
	store := newStore(t)
	storeSpec := storeSpecForStringWithinTransport(store)

	for _, c := range validReferenceTestCases {
		ref, err := Transport.ParseReference(c.input)
		require.NoError(t, err, c.input)
		assert.Equal(t, storeSpec+c.canonical, ref.StringWithinTransport(), c.input)
	}
}

func TestStorageReferencePolicyConfigurationIdentity(t *testing.T) {
	store := newStore(t)
	storeSpec := fmt.Sprintf("[%s@%s]", store.GraphDriverName(), store.GraphRoot())

	for _, c := range validReferenceTestCases {
		ref, err := Transport.ParseReference(c.input)
		require.NoError(t, err, c.input)
		assert.Equal(t, storeSpec+c.canonical, ref.PolicyConfigurationIdentity(), c.input)
	}
}

func TestStorageReferencePolicyConfigurationNamespaces(t *testing.T) {
	store := newStore(t)
	storeSpec := fmt.Sprintf("[%s@%s]", store.GraphDriverName(), store.GraphRoot())

	for _, c := range validReferenceTestCases {
		ref, err := Transport.ParseReference(c.input)
		require.NoError(t, err, c.input)
		expectedNS := []string{}
		for _, ns := range c.namespaces {
			expectedNS = append(expectedNS, storeSpec+ns)
		}
		expectedNS = append(expectedNS, storeSpec)
		expectedNS = append(expectedNS, fmt.Sprintf("[%s]", store.GraphRoot()))
		assert.Equal(t, expectedNS, ref.PolicyConfigurationNamespaces(), c.input)
	}
}

// NewImage, NewImageSource, NewImageDestination, DeleteImage tested in storage_test.go

func TestResolveReference(t *testing.T) {
	// This is, so far, only a minimal smoke test

	ensureTestCanCreateImages(t)

	store := newStore(t)
	storeSpec := storeSpecForStringWithinTransport(store)
	cache := memory.New()

	id := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	// Create an image with a known name and ID
	ref, err := Transport.ParseStoreReference(store, "test@"+id)
	require.NoError(t, err)
	createImage(t, ref, cache, []testBlob{makeLayer(t, archive.Gzip)}, nil)

	for _, c := range []struct {
		input    string
		expected string // "" on error
	}{
		{ // No ID match
			"@bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "",
		},
		{"@" + id, "@" + id},                                  // ID-only lookup
		{"test", "docker.io/library/test:latest@" + id},       // Name is resolved to include ID
		{"nottest", ""},                                       // No name match
		{"test@" + id, "docker.io/library/test:latest@" + id}, // Name+ID works, and is unchanged
		{"nottest@" + id, ""},                                 // Name mismatch is rejected even with an ID
	} {
		input, err := Transport.ParseStoreReference(store, c.input)
		require.NoError(t, err, c.input)
		inputClone := *input
		resolved, img, err := ResolveReference(input)
		if c.expected == "" {
			assert.Error(t, err, c.input)
		} else {
			require.NoError(t, err, c.input)
			require.Equal(t, &inputClone, input) // input was not modified in-place
			assert.Equal(t, id, img.ID, c.input)
			assert.Equal(t, storeSpec+c.expected, resolved.StringWithinTransport(), c.input)
		}
	}
}

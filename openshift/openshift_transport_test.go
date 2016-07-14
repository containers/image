package openshift

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	sha256digestHex = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	sha256digest    = "@sha256:" + sha256digestHex
)

func TestTransportName(t *testing.T) {
	assert.Equal(t, "atomic", Transport.Name())
}

// Transport.ParseReference, ParseReference untested because they depend
// on per-user configuration.
var testBaseURL *url.URL

func init() {
	u, err := url.Parse("https://registry.example.com:8443")
	if err != nil {
		panic("Error initializing testBaseURL")
	}
	testBaseURL = u
}

func TestNewReference(t *testing.T) {
	// Success
	ref, err := NewReference(testBaseURL, "ns", "stream", "notlatest")
	require.NoError(t, err)
	osRef, ok := ref.(openshiftReference)
	require.True(t, ok)
	assert.Equal(t, testBaseURL.String(), osRef.baseURL.String())
	assert.Equal(t, "ns", osRef.namespace)
	assert.Equal(t, "stream", osRef.stream)
	assert.Equal(t, "notlatest", osRef.tag)
	assert.Equal(t, "registry.example.com:8443/ns/stream:notlatest", osRef.dockerReference.String())

	// Components creating an invalid Docker Reference name
	_, err = NewReference(testBaseURL, "ns", "UPPERCASEISINVALID", "notlatest")
	assert.Error(t, err)

	_, err = NewReference(testBaseURL, "ns", "stream", "invalid!tag@value=")
	assert.Error(t, err)
}

func TestReferenceTransport(t *testing.T) {
	ref, err := NewReference(testBaseURL, "ns", "stream", "notlatest")
	require.NoError(t, err)
	assert.Equal(t, Transport, ref.Transport())
}

func TestReferenceStringWithinTransport(t *testing.T) {
	ref, err := NewReference(testBaseURL, "ns", "stream", "notlatest")
	require.NoError(t, err)
	assert.Equal(t, "ns/stream:notlatest", ref.StringWithinTransport())
	// We should do one more round to verify that the output can be parsed, to an equal value,
	// but that is untested because it depends on per-user configuration.
}

func TestReferenceNewImage(t *testing.T) {
	ref, err := NewReference(testBaseURL, "ns", "stream", "notlatest")
	require.NoError(t, err)
	_, err = ref.NewImage("", true)
	assert.Error(t, err)
}

// openshfitReference.NewImageSource, openshfitReference.NewImageDestination untested because they depend
// on per-user configuration when initializing httpClient.

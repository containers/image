package signature

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSimpleSigningFromBlob(t *testing.T) {
	data := []byte("some contents")

	sig := SimpleSigningFromBlob(data)
	assert.Equal(t, SimpleSigning{untrustedSignature: data}, sig)
}

func TestSimpleSigningFormatID(t *testing.T) {
	sig := SimpleSigningFromBlob([]byte("some contents"))
	assert.Equal(t, SimpleSigningFormat, sig.FormatID())
}

func TestSimpleSigningBlobChunk(t *testing.T) {
	data := []byte("some contents")

	sig := SimpleSigningFromBlob(data)
	chunk, err := sig.blobChunk()
	require.NoError(t, err)
	assert.Equal(t, data, chunk)
}

func TestSimpleSigningUntrustedSignature(t *testing.T) {
	data := []byte("some contents")

	sig := SimpleSigningFromBlob(data)
	assert.Equal(t, data, sig.UntrustedSignature())
}

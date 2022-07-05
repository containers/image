package signature

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlobSimpleSigning(t *testing.T) {
	simpleSigData, err := os.ReadFile("testdata/simple.signature")
	require.NoError(t, err)
	simpleSig := SimpleSigningFromBlob(simpleSigData)

	simpleBlob, err := Blob(simpleSig)
	require.NoError(t, err)
	assert.Equal(t, simpleSigData, simpleBlob)

	fromBlob, err := FromBlob(simpleBlob)
	require.NoError(t, err)
	fromBlobSimple, ok := fromBlob.(SimpleSigning)
	require.True(t, ok)
	assert.Equal(t, simpleSigData, fromBlobSimple.UntrustedSignature())
}

// mockFormatSignature returns a specified format
type mockFormatSignature struct {
	fmt FormatID
}

func (ms mockFormatSignature) FormatID() FormatID {
	return ms.fmt
}

func (ms mockFormatSignature) blobChunk() ([]byte, error) {
	panic("Unexpected call to a mock function")
}

func TestUnsuportedFormatError(t *testing.T) {
	// Warning: The exact text returned by the function is not an API commitment.
	for _, c := range []struct {
		input    Signature
		expected string
	}{
		{SimpleSigningFromBlob(nil), "unsupported signature format simple-signing"},
		{mockFormatSignature{CosignFormat}, "unsupported signature format cosign-json"},
		{mockFormatSignature{FormatID("invalid")}, `unsupported, and unrecognized, signature format "invalid"`},
	} {
		res := UnsupportedFormatError(c.input)
		assert.Equal(t, c.expected, res.Error(), string(c.input.FormatID()))
	}
}

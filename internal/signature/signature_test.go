package signature

import (
	"bytes"
	"fmt"
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

	// Using the newer format is accepted as well.
	fromBlob, err = FromBlob(append([]byte("\x00simple-signing\n"), simpleSigData...))
	require.NoError(t, err)
	fromBlobSimple, ok = fromBlob.(SimpleSigning)
	require.True(t, ok)
	assert.Equal(t, simpleSigData, fromBlobSimple.UntrustedSignature())

}

func TestBlobCosign(t *testing.T) {
	cosignSig := CosignFromComponents("mime-type", []byte("payload"),
		map[string]string{"a": "b", "c": "d"})

	cosignBlob, err := Blob(cosignSig)
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(cosignBlob, []byte("\x00cosign-json\n{")))

	fromBlob, err := FromBlob(cosignBlob)
	require.NoError(t, err)
	fromBlobCosign, ok := fromBlob.(Cosign)
	require.True(t, ok)
	assert.Equal(t, cosignSig.UntrustedMIMEType(), fromBlobCosign.UntrustedMIMEType())
	assert.Equal(t, cosignSig.UntrustedPayload(), fromBlobCosign.UntrustedPayload())
	assert.Equal(t, cosignSig.UntrustedAnnotations(), fromBlobCosign.UntrustedAnnotations())
}

func TestFromBlobInvalid(t *testing.T) {
	// Round-tripping valid data has been tested in TestBlobSimpleSigning and TestBlobCosign above.
	for _, c := range []string{
		"",                          // Empty
		"\xFFsimple-signing\nhello", // Invalid first byte
		"\x00simple-signing",        // No newline
		"\x00format\xFFname\ndata",  // Non-ASCII format value
		"\x00unknown-format\ndata",  // Unknown format
	} {
		_, err := FromBlob([]byte(c))
		assert.Error(t, err, fmt.Sprintf("%#v", c))
	}
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
		{CosignFromComponents("mime-type", nil, nil), "unsupported signature format cosign-json"},
		{mockFormatSignature{FormatID("invalid")}, `unsupported, and unrecognized, signature format "invalid"`},
	} {
		res := UnsupportedFormatError(c.input)
		assert.Equal(t, c.expected, res.Error(), string(c.input.FormatID()))
	}
}

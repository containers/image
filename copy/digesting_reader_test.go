package copy

import (
	"bytes"
	"io"
	"testing"

	digest "github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDigestingReader(t *testing.T) {
	// Only the failure cases, success is tested in TestDigestingReaderRead below.
	source := bytes.NewReader([]byte("abc"))
	for _, input := range []digest.Digest{
		"abc",             // Not algo:hexvalue
		"crc32:",          // Unknown algorithm, empty value
		"crc32:012345678", // Unknown algorithm
		"sha256:",         // Empty value
		"sha256:0",        // Invalid hex value
		"sha256:01",       // Invalid length of hex value
	} {
		_, err := newDigestingReader(source, input)
		assert.Error(t, err, input.String())
	}
}

func TestDigestingReaderRead(t *testing.T) {
	cases := []struct {
		input  []byte
		digest digest.Digest
	}{
		{[]byte(""), "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{[]byte("abc"), "sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"},
		{make([]byte, 65537), "sha256:3266304f31be278d06c3bd3eb9aa3e00c59bedec0a890de466568b0b90b0e01f"},
	}
	// Valid input
	for _, c := range cases {
		source := bytes.NewReader(c.input)
		reader, err := newDigestingReader(source, c.digest)
		require.NoError(t, err, c.digest.String())
		dest := bytes.Buffer{}
		n, err := io.Copy(&dest, reader)
		assert.NoError(t, err, c.digest.String())
		assert.Equal(t, int64(len(c.input)), n, c.digest.String())
		assert.Equal(t, c.input, dest.Bytes(), c.digest.String())
		assert.False(t, reader.validationFailed, c.digest.String())
		assert.True(t, reader.validationSucceeded, c.digest.String())
	}
	// Modified input
	for _, c := range cases {
		source := bytes.NewReader(bytes.Join([][]byte{c.input, []byte("x")}, nil))
		reader, err := newDigestingReader(source, c.digest)
		require.NoError(t, err, c.digest.String())
		dest := bytes.Buffer{}
		_, err = io.Copy(&dest, reader)
		assert.Error(t, err, c.digest.String())
		assert.True(t, reader.validationFailed, c.digest.String())
		assert.False(t, reader.validationSucceeded, c.digest.String())
	}
	// Truncated input
	for _, c := range cases {
		source := bytes.NewReader(c.input)
		reader, err := newDigestingReader(source, c.digest)
		require.NoError(t, err, c.digest.String())
		if len(c.input) != 0 {
			dest := bytes.Buffer{}
			truncatedLen := int64(len(c.input) - 1)
			n, err := io.CopyN(&dest, reader, truncatedLen)
			assert.NoError(t, err, c.digest.String())
			assert.Equal(t, truncatedLen, n, c.digest.String())
		}
		assert.False(t, reader.validationFailed, c.digest.String())
		assert.False(t, reader.validationSucceeded, c.digest.String())
	}
}

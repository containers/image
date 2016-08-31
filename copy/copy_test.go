package copy

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDigestingReader(t *testing.T) {
	// Only the failure cases, success is tested in TestDigestingReaderRead below.
	source := bytes.NewReader([]byte("abc"))
	for _, input := range []string{
		"abc",             // Not algo:hexvalue
		"crc32:",          // Unknown algorithm, empty value
		"crc32:012345678", // Unknown algorithm
		"sha256:",         // Empty value
		"sha256:0",        // Invalid hex value
		"sha256:01",       // Invalid length of hex value
	} {
		validationFailed := false
		_, err := newDigestingReader(source, input, &validationFailed)
		assert.Error(t, err, input)
	}
}

func TestDigestingReaderRead(t *testing.T) {
	cases := []struct {
		input  []byte
		digest string
	}{
		{[]byte(""), "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{[]byte("abc"), "sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"},
		{make([]byte, 65537, 65537), "sha256:3266304f31be278d06c3bd3eb9aa3e00c59bedec0a890de466568b0b90b0e01f"},
	}
	// Valid input
	for _, c := range cases {
		source := bytes.NewReader(c.input)
		validationFailed := false
		reader, err := newDigestingReader(source, c.digest, &validationFailed)
		require.NoError(t, err, c.digest)
		dest := bytes.Buffer{}
		n, err := io.Copy(&dest, reader)
		assert.NoError(t, err, c.digest)
		assert.Equal(t, int64(len(c.input)), n, c.digest)
		assert.Equal(t, c.input, dest.Bytes(), c.digest)
		assert.False(t, validationFailed, c.digest)
	}
	// Modified input
	for _, c := range cases {
		source := bytes.NewReader(bytes.Join([][]byte{c.input, []byte("x")}, nil))
		validationFailed := false
		reader, err := newDigestingReader(source, c.digest, &validationFailed)
		require.NoError(t, err, c.digest)
		dest := bytes.Buffer{}
		_, err = io.Copy(&dest, reader)
		assert.Error(t, err, c.digest)
		assert.True(t, validationFailed)
	}
}

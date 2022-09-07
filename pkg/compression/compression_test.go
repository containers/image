package compression

import (
	"bytes"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectCompression(t *testing.T) {
	cases := []string{
		"fixtures/Hello.uncompressed",
		"fixtures/Hello.gz",
		"fixtures/Hello.bz2",
		"fixtures/Hello.xz",
		"fixtures/Hello.zst",
	}

	// The original stream is preserved.
	for _, c := range cases {
		originalContents, err := os.ReadFile(c)
		require.NoError(t, err, c)

		stream, err := os.Open(c)
		require.NoError(t, err, c)
		defer stream.Close()

		_, updatedStream, err := DetectCompression(stream)
		require.NoError(t, err, c)

		updatedContents, err := io.ReadAll(updatedStream)
		require.NoError(t, err, c)
		assert.Equal(t, originalContents, updatedContents, c)
	}

	// The correct decompressor is chosen, and the result is as expected.
	for _, c := range cases {
		stream, err := os.Open(c)
		require.NoError(t, err, c)
		defer stream.Close()

		decompressor, updatedStream, err := DetectCompression(stream)
		require.NoError(t, err, c)

		var uncompressedStream io.Reader
		if decompressor == nil {
			uncompressedStream = updatedStream
		} else {
			s, err := decompressor(updatedStream)
			require.NoError(t, err)
			defer s.Close()
			uncompressedStream = s
		}

		uncompressedContents, err := io.ReadAll(uncompressedStream)
		require.NoError(t, err, c)
		assert.Equal(t, []byte("Hello"), uncompressedContents, c)
	}

	// Empty input is handled reasonably.
	decompressor, updatedStream, err := DetectCompression(bytes.NewReader([]byte{}))
	require.NoError(t, err)
	assert.Nil(t, decompressor)
	updatedContents, err := io.ReadAll(updatedStream)
	require.NoError(t, err)
	assert.Equal(t, []byte{}, updatedContents)

	// Error reading input
	reader, writer := io.Pipe()
	defer reader.Close()
	err = writer.CloseWithError(errors.New("Expected error reading input in DetectCompression"))
	assert.NoError(t, err)
	_, _, err = DetectCompression(reader)
	assert.Error(t, err)
}

func TestAutoDecompress(t *testing.T) {
	cases := []struct {
		filename     string
		isCompressed bool
	}{
		{"fixtures/Hello.uncompressed", false},
		{"fixtures/Hello.gz", true},
		{"fixtures/Hello.bz2", true},
		{"fixtures/Hello.xz", true},
	}

	// The correct decompressor is chosen, and the result is as expected.
	for _, c := range cases {
		stream, err := os.Open(c.filename)
		require.NoError(t, err, c.filename)
		defer stream.Close()

		uncompressedStream, isCompressed, err := AutoDecompress(stream)
		require.NoError(t, err, c.filename)
		defer uncompressedStream.Close()

		assert.Equal(t, c.isCompressed, isCompressed)

		uncompressedContents, err := io.ReadAll(uncompressedStream)
		require.NoError(t, err, c.filename)
		assert.Equal(t, []byte("Hello"), uncompressedContents, c.filename)
	}

	// Empty input is handled reasonably.
	uncompressedStream, isCompressed, err := AutoDecompress(bytes.NewReader([]byte{}))
	require.NoError(t, err)
	assert.False(t, isCompressed)
	uncompressedContents, err := io.ReadAll(uncompressedStream)
	require.NoError(t, err)
	assert.Equal(t, []byte{}, uncompressedContents)

	// Error initializing a decompressor (for a detected format)
	_, _, err = AutoDecompress(bytes.NewReader([]byte{0xFD, 0x37, 0x7A, 0x58, 0x5A, 0x00}))
	assert.Error(t, err)

	// Error reading input
	reader, writer := io.Pipe()
	defer reader.Close()
	err = writer.CloseWithError(errors.New("Expected error reading input in AutoDecompress"))
	require.NoError(t, err)
	_, _, err = AutoDecompress(reader)
	assert.Error(t, err)
}

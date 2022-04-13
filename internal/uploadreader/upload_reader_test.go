package uploadreader

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUploadReader(t *testing.T) {
	// This is a smoke test in a single goroutine, without really testing the locking.

	data := bytes.Repeat([]byte{0x01}, 65535)
	// No termination
	ur := NewUploadReader(bytes.NewReader(data))
	read, err := io.ReadAll(ur)
	require.NoError(t, err)
	assert.Equal(t, data, read)

	// Terminated
	ur = NewUploadReader(bytes.NewReader(data))
	readLen := len(data) / 2
	read, err = io.ReadAll(io.LimitReader(ur, int64(readLen)))
	require.NoError(t, err)
	assert.Equal(t, data[:readLen], read)
	terminationErr := errors.New("Terminated")
	ur.Terminate(terminationErr)
	read, err = io.ReadAll(ur)
	assert.Equal(t, terminationErr, err)
	assert.Len(t, read, 0)
}

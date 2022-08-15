package blobcache

import (
	"bytes"
	"io"
	"testing"

	"github.com/containers/image/v5/internal/private"
	"github.com/stretchr/testify/assert"
)

func readNextStream(streams chan io.ReadCloser, errs chan error) ([]byte, error) {
	select {
	case r := <-streams:
		if r == nil {
			return nil, nil
		}
		defer r.Close()
		return io.ReadAll(r)
	case err := <-errs:
		return nil, err
	}
}

// readSeekerNopCloser adds a no-op Close() method to a readSeeker
type readSeekerNopCloser struct {
	io.ReadSeeker
}

func (c *readSeekerNopCloser) Close() error {
	return nil
}

func TestStreamChunksFromFile(t *testing.T) {
	file := &readSeekerNopCloser{bytes.NewReader([]byte("123456789"))}
	streams := make(chan io.ReadCloser)
	errs := make(chan error)
	chunks := []private.ImageSourceChunk{
		{Offset: 1, Length: 2},
		{Offset: 4, Length: 1},
	}
	go streamChunksFromFile(streams, errs, file, chunks)

	for _, c := range []struct {
		expectedData  []byte
		expectedError error
	}{
		{[]byte("23"), nil},
		{[]byte("5"), nil},
		{[]byte(nil), nil},
	} {
		data, err := readNextStream(streams, errs)
		assert.Equal(t, c.expectedData, data)
		assert.Equal(t, c.expectedError, err)
	}
}

package copy

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/containers/image/v5/types"
	"github.com/stretchr/testify/assert"
)

func newSUT(
	t *testing.T,
	reader io.Reader,
	duration time.Duration,
	channel chan types.ProgressProperties,
) *progressReader {
	artifact := types.BlobInfo{Size: 10}

	go func() {
		res := <-channel
		assert.Equal(t, res.Event, types.ProgressEventNewArtifact)
		assert.Equal(t, res.Artifact, artifact)
	}()
	res := newProgressReader(reader, channel, duration, artifact)

	return res
}

func TestNewProgressReader(t *testing.T) {
	// Given
	channel := make(chan types.ProgressProperties)
	sut := newSUT(t, nil, time.Second, channel)
	assert.NotNil(t, sut)

	// When/Then
	go func() {
		res := <-channel
		assert.Equal(t, res.Event, types.ProgressEventDone)
	}()
	sut.reportDone()
}

func TestReadWithoutEvent(t *testing.T) {
	// Given
	channel := make(chan types.ProgressProperties)
	reader := bytes.NewReader([]byte{0, 1, 2})
	sut := newSUT(t, reader, time.Second, channel)
	assert.NotNil(t, sut)

	// When
	b := []byte{0, 1, 2, 3, 4}
	read, err := reader.Read(b)

	// Then
	assert.Nil(t, err)
	assert.Equal(t, read, 3)
}

func TestReadWithEvent(t *testing.T) {
	// Given
	channel := make(chan types.ProgressProperties)
	reader := bytes.NewReader([]byte{0, 1, 2, 3, 4, 5, 6})
	sut := newSUT(t, reader, time.Nanosecond, channel)
	assert.NotNil(t, sut)
	b := []byte{0, 1, 2, 3, 4}

	// When/Then
	go func() {
		res := <-channel
		assert.Equal(t, res.Event, types.ProgressEventRead)
		assert.Equal(t, res.Offset, uint64(5))
		assert.Equal(t, res.OffsetUpdate, uint64(5))
	}()
	read, err := reader.Read(b)
	assert.Equal(t, read, 5)
	assert.Nil(t, err)

}

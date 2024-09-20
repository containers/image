package copy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vbauerster/mpb/v8/decor"
)

func TestCustomPartialBlobDecorFunc(t *testing.T) {
	// A stub test
	s := decor.Statistics{}
	assert.Equal(t, "0.0b / 0.0b (skipped: 0.0b)", customPartialBlobDecorFunc(s))
	// Partial pull in progress
	s = decor.Statistics{}
	s.Current = 1097653
	s.Total = 8329917
	s.Refill = 509722
	assert.Equal(t, "1.0MiB / 7.9MiB (skipped: 497.8KiB = 6.12%)", customPartialBlobDecorFunc(s))
	// Almost complete, but no reuse
	s.Current = int64(float64(s.Total) * 0.95)
	s.Refill = 0
	assert.Equal(t, "7.5MiB / 7.9MiB (skipped: 0.0b = 0.00%)", customPartialBlobDecorFunc(s))
}

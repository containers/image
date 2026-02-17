package copy

import (
	"fmt"
	"io"
	"time"

	"github.com/containers/image/v5/types"
	"github.com/vbauerster/mpb/v8/decor"
)

const (
	// nonTTYProgressChannelSize is the buffer size for the progress channel
	// in non-TTY mode. Buffered to prevent blocking during parallel downloads.
	nonTTYProgressChannelSize = 10

	// nonTTYProgressInterval is how often aggregate progress is printed
	// in non-TTY mode.
	nonTTYProgressInterval = 500 * time.Millisecond
)

// nonTTYProgressWriter consumes ProgressProperties from a channel and writes
// aggregate text-based progress output suitable for non-TTY environments.
// No mutex needed - single goroutine processes events sequentially from channel.
type nonTTYProgressWriter struct {
	output io.Writer

	// Aggregate tracking (no per-blob state needed)
	totalSize  int64 // Sum of all known blob sizes
	downloaded int64 // Total bytes downloaded (accumulated from OffsetUpdate)

	// Output throttling
	lastOutput     time.Time
	outputInterval time.Duration
}

// newNonTTYProgressWriter creates a writer that outputs aggregate download
// progress as simple text lines, suitable for non-TTY environments like
// CI/CD pipelines or redirected output.
func newNonTTYProgressWriter(output io.Writer, interval time.Duration) *nonTTYProgressWriter {
	return &nonTTYProgressWriter{
		output:         output,
		outputInterval: interval,
	}
}

// setupNonTTYProgressWriter configures text-based progress output for non-TTY
// environments unless the caller already provided a buffered Progress channel.
// Returns a cleanup function that must be deferred by the caller.
func setupNonTTYProgressWriter(reportWriter io.Writer, options *Options) func() {
	if options.Progress != nil && cap(options.Progress) > 0 {
		return func() {}
	}

	// Use user's interval if greater than our default, otherwise use default.
	// This allows users to slow down output while maintaining a sensible minimum.
	interval := max(options.ProgressInterval, nonTTYProgressInterval)
	if options.ProgressInterval <= 0 {
		options.ProgressInterval = nonTTYProgressInterval
	}

	progressChan := make(chan types.ProgressProperties, nonTTYProgressChannelSize)
	options.Progress = progressChan

	pw := newNonTTYProgressWriter(reportWriter, interval)
	go pw.Run(progressChan)

	return func() { close(progressChan) }
}

// Run consumes progress events from the channel and prints throttled
// aggregate progress. Blocks until the channel is closed. Intended to
// be called as a goroutine: go tw.Run(progressChan)
func (w *nonTTYProgressWriter) Run(ch <-chan types.ProgressProperties) {
	for props := range ch {
		switch props.Event {
		case types.ProgressEventNewArtifact:
			// New blob starting - add its size to total
			w.totalSize += props.Artifact.Size

		case types.ProgressEventRead:
			// Bytes downloaded - accumulate and maybe print
			w.downloaded += int64(props.OffsetUpdate)
			if time.Since(w.lastOutput) > w.outputInterval {
				fmt.Fprintf(w.output, "Progress: %.1f / %.1f\n",
					decor.SizeB1024(w.downloaded), decor.SizeB1024(w.totalSize))
				w.lastOutput = time.Now()
			}
		}
	}
}

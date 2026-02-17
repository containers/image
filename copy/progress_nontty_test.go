package copy

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/containers/image/v5/types"
	"github.com/stretchr/testify/assert"
)

func TestNonTTYProgressWriterRun(t *testing.T) {
	tests := []struct {
		name           string
		interval       time.Duration
		events         []types.ProgressProperties
		wantTotalSize  int64
		wantDownloaded int64
		wantLines      int
		wantContains   string
	}{
		{
			name:     "new artifacts only",
			interval: time.Nanosecond,
			events: []types.ProgressProperties{
				{Event: types.ProgressEventNewArtifact, Artifact: types.BlobInfo{Size: 1024}},
				{Event: types.ProgressEventNewArtifact, Artifact: types.BlobInfo{Size: 2048}},
			},
			wantTotalSize:  3072,
			wantDownloaded: 0,
			wantLines:      0,
		},
		{
			name:     "read events produce output",
			interval: time.Nanosecond,
			events: []types.ProgressProperties{
				{Event: types.ProgressEventNewArtifact, Artifact: types.BlobInfo{Size: 10240}},
				{Event: types.ProgressEventRead, OffsetUpdate: 5120},
			},
			wantTotalSize:  10240,
			wantDownloaded: 5120,
			wantLines:      1,
			wantContains:   "Progress:",
		},
		{
			name:     "throttling limits output",
			interval: 5 * time.Second,
			events: []types.ProgressProperties{
				{Event: types.ProgressEventNewArtifact, Artifact: types.BlobInfo{Size: 10240}},
				{Event: types.ProgressEventRead, OffsetUpdate: 1024},
				{Event: types.ProgressEventRead, OffsetUpdate: 1024},
				{Event: types.ProgressEventRead, OffsetUpdate: 1024},
			},
			wantTotalSize:  10240,
			wantDownloaded: 3072,
			wantLines:      1,
		},
		{
			name:     "unknown events ignored",
			interval: time.Nanosecond,
			events: []types.ProgressProperties{
				{Event: types.ProgressEventDone},
			},
			wantTotalSize:  0,
			wantDownloaded: 0,
			wantLines:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			pw := newNonTTYProgressWriter(&buf, tt.interval)

			ch := make(chan types.ProgressProperties, len(tt.events))
			for _, e := range tt.events {
				ch <- e
			}
			close(ch)

			pw.Run(ch)

			assert.Equal(t, tt.wantTotalSize, pw.totalSize)
			assert.Equal(t, tt.wantDownloaded, pw.downloaded)

			output := buf.String()
			if tt.wantLines == 0 {
				assert.Empty(t, output)
			} else {
				lines := strings.Split(strings.TrimSpace(output), "\n")
				assert.Equal(t, tt.wantLines, len(lines))
			}
			if tt.wantContains != "" {
				assert.Contains(t, output, tt.wantContains)
			}
		})
	}
}

func TestSetupNonTTYProgressWriter(t *testing.T) {
	tests := []struct {
		name                 string
		progress             chan types.ProgressProperties
		progressInterval     time.Duration
		wantProgressSet      bool
		wantIntervalSet      bool
		wantMinInterval      time.Duration
	}{
		{
			name:            "nil channel gets default setup",
			progress:        nil,
			progressInterval: 0,
			wantProgressSet: true,
			wantIntervalSet: true,
			wantMinInterval: nonTTYProgressInterval,
		},
		{
			name:            "unbuffered channel gets replaced",
			progress:        make(chan types.ProgressProperties),
			progressInterval: 0,
			wantProgressSet: true,
			wantIntervalSet: true,
			wantMinInterval: nonTTYProgressInterval,
		},
		{
			name:            "buffered channel is kept",
			progress:        make(chan types.ProgressProperties, 5),
			progressInterval: 0,
			wantProgressSet: false,
			wantIntervalSet: false,
		},
		{
			name:            "caller interval larger than default is respected",
			progress:        nil,
			progressInterval: 2 * time.Second,
			wantProgressSet: true,
			wantIntervalSet: false,
			wantMinInterval: 2 * time.Second,
		},
		{
			name:             "caller interval smaller than default is kept",
			progress:         nil,
			progressInterval: 100 * time.Millisecond,
			wantProgressSet:  true,
			wantIntervalSet:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			opts := &Options{
				Progress:         tt.progress,
				ProgressInterval: tt.progressInterval,
			}
			originalProgress := opts.Progress

			cleanup := setupNonTTYProgressWriter(&buf, opts)
			defer cleanup()

			if tt.wantProgressSet {
				assert.NotNil(t, opts.Progress)
				assert.Greater(t, cap(opts.Progress), 0)
				if originalProgress != nil {
					assert.NotEqual(t, originalProgress, opts.Progress)
				}
			} else {
				assert.Equal(t, originalProgress, opts.Progress)
			}

			if tt.wantIntervalSet {
				assert.Equal(t, nonTTYProgressInterval, opts.ProgressInterval)
			}

			if tt.wantMinInterval > 0 {
				assert.GreaterOrEqual(t, opts.ProgressInterval, tt.wantMinInterval)
			}
		})
	}
}

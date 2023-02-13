package docker

import (
	"errors"
	"math"
	"net/http"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDecimalInString(t *testing.T) {
	for _, prefix := range []string{"", "text", "0"} {
		for _, suffix := range []string{"", "text"} {
			for _, c := range []struct {
				s string
				v int64
			}{
				{"0", 0},
				{"1", 1},
				{"0700", 700}, // not octal
			} {
				input := prefix + c.s + suffix
				res, pos, err := parseDecimalInString(input, len(prefix))
				require.NoError(t, err, input)
				assert.Equal(t, c.v, res, input)
				assert.Equal(t, len(prefix)+len(c.s), pos, input)
			}
			for _, c := range []string{
				"-1",
				"xA",
				"&",
				"",
				"999999999999999999999999999999999999999999999999999999999999999999",
			} {
				input := prefix + c + suffix
				_, _, err := parseDecimalInString(input, len(prefix))
				assert.Error(t, err, c)
			}
		}
	}
}

func TestParseExpectedChar(t *testing.T) {
	for _, prefix := range []string{"", "text", "0"} {
		for _, suffix := range []string{"", "text"} {
			input := prefix + "+" + suffix
			pos, err := parseExpectedChar(input, len(prefix), '+')
			require.NoError(t, err, input)
			assert.Equal(t, len(prefix)+1, pos, input)

			_, err = parseExpectedChar(input, len(prefix), '-')
			assert.Error(t, err, input)
		}
	}
}

func TestParseContentRange(t *testing.T) {
	for _, c := range []struct {
		in                          string
		first, last, completeLength int64
	}{
		{"bytes 0-0/1", 0, 0, 1},
		{"bytes 010-020/030", 10, 20, 30},
		{"bytes 1000-1010/*", 1000, 1010, -1},
	} {
		first, last, completeLength, err := parseContentRange(&http.Response{
			Header: http.Header{
				http.CanonicalHeaderKey("Content-Range"): []string{c.in},
			},
		})
		require.NoError(t, err, c.in)
		assert.Equal(t, c.first, first, c.in)
		assert.Equal(t, c.last, last, c.in)
		assert.Equal(t, c.completeLength, completeLength, c.in)
	}

	for _, hdr := range []http.Header{
		nil,
		{http.CanonicalHeaderKey("Content-Range"): []string{}},
		{http.CanonicalHeaderKey("Content-Range"): []string{"bytes 1-2/3", "bytes 1-2/3"}},
	} {
		_, _, _, err := parseContentRange(&http.Response{
			Header: hdr,
		})
		assert.Error(t, err)
	}

	for _, c := range []string{
		"",
		"notbytes 1-2/3",
		"bytes ",
		"bytes x-2/3",
		"bytes 1*2/3",
		"bytes 1",
		"bytes 1-",
		"bytes 1-x/3",
		"bytes 1-2",
		"bytes 1-2@3",
		"bytes 1-2/",
		"bytes 1-2/*a",
		"bytes 1-2/3a",
	} {
		_, _, _, err := parseContentRange(&http.Response{
			Header: http.Header{
				http.CanonicalHeaderKey("Content-Range"): []string{c},
			},
		})
		assert.Error(t, err, c, c)
	}
}

func TestMillisecondsSinceOptional(t *testing.T) {
	current := time.Date(2023, 2, 9, 8, 7, 6, 5, time.UTC)
	res := millisecondsSinceOptional(current, time.Time{})
	assert.True(t, math.IsNaN(res))
	tm := current.Add(-60 * time.Second) // 60 seconds _before_ current
	res = millisecondsSinceOptional(current, tm)
	assert.Equal(t, res, 60_000.0)
}

func TestBodyReaderErrorIfNotReconnecting(t *testing.T) {
	// Silence logrus.Info logs in the tested method
	prevLevel := logrus.StandardLogger().Level
	logrus.StandardLogger().SetLevel(logrus.WarnLevel)
	t.Cleanup(func() {
		logrus.StandardLogger().SetLevel(prevLevel)
	})

	for _, c := range []struct {
		name            string
		previousRetry   bool
		currentOffset   int64
		currentTime     int // milliseconds
		expectReconnect bool
	}{
		{
			name:            "A lot of progress, after a long time, second retry",
			previousRetry:   true,
			currentOffset:   2 * bodyReaderMinimumProgress,
			currentTime:     2 * bodyReaderMSSinceLastRetry,
			expectReconnect: true,
		},
		{
			name:            "A lot of progress, after little time, second retry",
			previousRetry:   true,
			currentOffset:   2 * bodyReaderMinimumProgress,
			currentTime:     1,
			expectReconnect: true,
		},
		{
			name:            "Little progress, after a long time, second retry",
			previousRetry:   true,
			currentOffset:   1,
			currentTime:     2 * bodyReaderMSSinceLastRetry,
			expectReconnect: true,
		},
		{
			name:            "Little progress, after little time, second retry",
			previousRetry:   true,
			currentOffset:   1,
			currentTime:     1,
			expectReconnect: false,
		},
		{
			name:            "Little progress, after little time, first retry",
			previousRetry:   false,
			currentOffset:   1,
			currentTime:     bodyReaderMSSinceLastRetry / 2,
			expectReconnect: true,
		},
	} {
		tm := time.Now()
		br := bodyReader{}
		if c.previousRetry {
			br.lastRetryOffset = 2 * bodyReaderMinimumProgress
			br.offset = br.lastRetryOffset + c.currentOffset
			br.firstConnectionTime = tm.Add(-time.Duration(c.currentTime+2*bodyReaderMSSinceLastRetry) * time.Millisecond)
			br.lastRetryTime = tm.Add(-time.Duration(c.currentTime) * time.Millisecond)
		} else {
			br.lastRetryOffset = -1
			br.lastRetryTime = time.Time{}
			br.offset = c.currentOffset
			br.firstConnectionTime = tm.Add(-time.Duration(c.currentTime) * time.Millisecond)
		}
		err := br.errorIfNotReconnecting(errors.New("some error for error text only"), "URL for error text only")
		if c.expectReconnect {
			assert.NoError(t, err, c.name, br)
		} else {
			assert.Error(t, err, c.name, br)
		}
	}
}

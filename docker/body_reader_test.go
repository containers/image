package docker

import (
	"net/http"
	"testing"

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

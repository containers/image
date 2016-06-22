package main

import (
	"io"
	"strings"

	"github.com/go-check/check"
)

// consumeAndLogOutput takes (f, err) from an exec.*Pipe(), and causes all output to it to be logged to c.
func consumeAndLogOutput(c *check.C, id string, f io.ReadCloser, err error) {
	c.Assert(err, check.IsNil)
	go func() {
		defer func() {
			f.Close()
			c.Logf("Output %s: Closed", id)
		}()
		buf := make([]byte, 1024)
		for {
			c.Logf("Output %s: waiting", id)
			n, err := f.Read(buf)
			c.Logf("Output %s: got %d,%#v: %s", id, n, err, strings.TrimSuffix(string(buf[:n]), "\n"))
			if n <= 0 {
				break
			}
		}
	}()
}

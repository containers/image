package main

import (
	"io"
	"os/exec"
	"strings"

	"github.com/go-check/check"
)

const skopeoBinary = "skopeo"

// consumeAndLogOutputStream takes (f, err) from an exec.*Pipe(), and causes all output to it to be logged to c.
func consumeAndLogOutputStream(c *check.C, id string, f io.ReadCloser, err error) {
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

// consumeAndLogOutputs causes all output to stdout and stderr from an *exec.Cmd to be logged to c
func consumeAndLogOutputs(c *check.C, id string, cmd *exec.Cmd) {
	stdout, err := cmd.StdoutPipe()
	consumeAndLogOutputStream(c, id+" stdout", stdout, err)
	stderr, err := cmd.StderrPipe()
	consumeAndLogOutputStream(c, id+" stderr", stderr, err)
}

// assertSkopeoSucceeds runs a skopeo command as if exec.Command().CombinedOutput, verifies that the exit status is 0,
// and optionally that the output matches a multi-line regexp if it is nonempty;
// or terminates c on failure
func assertSkopeoSucceeds(c *check.C, regexp string, args ...string) {
	c.Logf("Running %s %s", skopeoBinary, strings.Join(args, " "))
	out, err := exec.Command(skopeoBinary, args...).CombinedOutput()
	c.Assert(err, check.IsNil, check.Commentf("%s", out))
	if regexp != "" {
		c.Assert(string(out), check.Matches, "(?s)"+regexp) // (?s) : '.' will also match newlines
	}
}

// assertSkopeoFails runs a skopeo command as if exec.Command().CombinedOutput, verifies that the exit status is 0,
// and that the output matches a multi-line regexp;
// or terminates c on failure
func assertSkopeoFails(c *check.C, regexp string, args ...string) {
	c.Logf("Running %s %s", skopeoBinary, strings.Join(args, " "))
	out, err := exec.Command(skopeoBinary, args...).CombinedOutput()
	c.Assert(err, check.NotNil, check.Commentf("%s", out))
	c.Assert(string(out), check.Matches, "(?s)"+regexp) // (?s) : '.' will also match newlines
}

// runCommandWithInput runs a command as if exec.Command(), sending it the input to stdin,
// and verifies that the exit status is 0, or terminates c on failure.
func runCommandWithInput(c *check.C, input string, name string, args ...string) {
	c.Logf("Running %s %s", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	consumeAndLogOutputs(c, name+" "+strings.Join(args, " "), cmd)
	stdin, err := cmd.StdinPipe()
	c.Assert(err, check.IsNil)
	err = cmd.Start()
	c.Assert(err, check.IsNil)
	_, err = stdin.Write([]byte(input))
	c.Assert(err, check.IsNil)
	err = stdin.Close()
	c.Assert(err, check.IsNil)
	err = cmd.Wait()
	c.Assert(err, check.IsNil)
}

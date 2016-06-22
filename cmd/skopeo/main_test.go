package main

import "bytes"

// runSkopeo creates an app object and runs it with args, with an implied first "skopeo".
// Returns output intended for stdout and the returned error, if any.
func runSkopeo(args ...string) (string, error) {
	app := createApp()
	stdout := bytes.Buffer{}
	app.Writer = &stdout
	args = append([]string{"skopeo"}, args...)
	err := app.Run(args)
	return stdout.String(), err
}

//go:build containers_image_sequoia

package testcli

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
)

func CheckCliVersion(version string) error {
	return exec.Command("sq", "--cli-version", version, "version").Run()
}

func GenerateKey(dir string, email string) (string, error) {
	cmd := exec.Command("sq", "--home", dir, "key", "generate", "--userid", fmt.Sprintf("<%s>", email), "--own-key", "--without-password")
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}

	if err := cmd.Start(); err != nil {
		return "", err
	}

	output, err := io.ReadAll(stderr)
	if err != nil {
		return "", err
	}

	if err := cmd.Wait(); err != nil {
		return "", err
	}

	re := regexp.MustCompile("(?m)^ *Fingerprint: ([0-9A-F]+)")
	matches := re.FindSubmatch(output)
	if matches == nil {
		return "", errors.New("unable to extract fingerprint")
	}
	fingerprint := string(matches[1][:])
	return fingerprint, nil
}

func ExportCert(dir string, fingerprint string) ([]byte, error) {
	cmd := exec.Command("sq", "--home", dir, "cert", "export", "--cert", fingerprint)
	return cmd.Output()
}

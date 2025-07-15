//go:build containers_image_sequoia

package testcli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
)

func CheckCliVersion(version string) error {
	return exec.Command("sq", "--cli-version", version, "version").Run()
}

func GenerateKey(t *testing.T, dir string, email, passphrase string) (string, error) {
	args := []string{"--home", dir, "key", "generate", "--userid", fmt.Sprintf("<%s>", email), "--own-key"}
	if passphrase != "" {
		pwFile := filepath.Join(t.TempDir(), "passphrase")
		err := os.WriteFile(pwFile, []byte(passphrase), 0o600)
		if err != nil {
			return "", err
		}
		args = append(args, "--new-password-file", pwFile)
	} else {
		args = append(args, "--without-password")
	}
	cmd := exec.Command("sq", args...)
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

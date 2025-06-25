//go:build containers_image_sequoia

package sequoia

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func checkCliVersion(version string) error {
	return exec.Command("sq", "--cli-version", version, "version").Run()
}

func generateKey(dir string, email string) (string, error) {
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

func exportCert(dir string, fingerprint string) ([]byte, error) {
	cmd := exec.Command("sq", "--home", dir, "cert", "export", "--cert", fingerprint)
	return cmd.Output()
}

func TestNewMechanismFromDirectory(t *testing.T) {
	if err := checkCliVersion("1.3.0"); err != nil {
		t.Skipf("sq not usable: %v", err)
	}
	dir := t.TempDir()
	_, err := NewMechanismFromDirectory(dir)
	if err != nil {
		t.Fatalf("unable to initialize a mechanism: %v", err)
	}
	_, err = generateKey(dir, "foo@example.org")
	if err != nil {
		t.Fatalf("unable to generate key: %v", err)
	}
	_, err = NewMechanismFromDirectory(dir)
	if err != nil {
		t.Fatalf("unable to initialize a mechanism: %v", err)
	}
}

func TestNewEphemeralMechanism(t *testing.T) {
	if err := checkCliVersion("1.3.0"); err != nil {
		t.Skipf("sq not usable: %v", err)
	}
	dir := t.TempDir()
	fingerprint, err := generateKey(dir, "foo@example.org")
	if err != nil {
		t.Fatalf("unable to generate key: %v", err)
	}
	output, err := exportCert(dir, fingerprint)
	if err != nil {
		t.Fatalf("unable to export cert: %v", err)
	}
	m, err := NewEphemeralMechanism()
	if err != nil {
		t.Fatalf("unable to initialize a mechanism: %v", err)
	}
	keyIdentities, err := m.ImportKeys(output)
	if err != nil {
		t.Fatalf("unable to import keys: %v", err)
	}
	if len(keyIdentities) != 1 || keyIdentities[0] != fingerprint {
		t.Fatalf("keyIdentity differ from the original: %v != %v",
			keyIdentities[0], fingerprint)
	}
}

func TestGenerateSignVerify(t *testing.T) {
	if err := checkCliVersion("1.3.0"); err != nil {
		t.Skipf("sq not usable: %v", err)
	}
	dir := t.TempDir()
	fingerprint, err := generateKey(dir, "foo@example.org")
	if err != nil {
		t.Fatalf("unable to generate key: %v", err)
	}
	m, err := NewMechanismFromDirectory(dir)
	if err != nil {
		t.Fatalf("unable to initialize a mechanism: %v", err)
	}
	input := []byte("Hello, world!")
	sig, err := m.Sign(input, fingerprint)
	if err != nil {
		t.Fatalf("unable to sign: %v", err)
	}
	contents, keyIdentity, err := m.Verify(sig)
	if err != nil {
		t.Fatalf("unable to verify: %v", err)
	}
	if !bytes.Equal(contents, input) {
		t.Fatalf("contents differ from the original")
	}
	if keyIdentity != fingerprint {
		t.Fatalf("keyIdentity differ from the original")
	}
}

func TestSignThenVerifyEphemeral(t *testing.T) {
	if err := checkCliVersion("1.3.0"); err != nil {
		t.Skipf("sq not usable: %v", err)
	}
	dir := t.TempDir()
	fingerprint, err := generateKey(dir, "foo@example.org")
	require.NoError(t, err)
	publicKey, err := exportCert(dir, fingerprint)
	require.NoError(t, err)
	m1, err := NewMechanismFromDirectory(dir)
	require.NoError(t, err)
	defer m1.Close()

	input := []byte("Hello, world!")
	sig, err := m1.Sign(input, fingerprint)
	require.NoError(t, err)

	m2, err := NewEphemeralMechanism()
	require.NoError(t, err)
	defer m2.Close()

	_, _, err = m2.Verify(sig) // With no public key, verification should fail
	assert.Error(t, err)

	keyIdentities, err := m2.ImportKeys(publicKey)
	require.NoError(t, err)
	require.Len(t, keyIdentities, 1)
	require.Equal(t, fingerprint, keyIdentities[0])
	contents, keyIdentity, err := m2.Verify(sig)
	require.NoError(t, err)
	assert.Equal(t, input, contents)
	assert.Equal(t, keyIdentity, fingerprint)
}

func TestMain(m *testing.M) {
	err := Init()
	if err != nil {
		panic(err)
	}
	status := m.Run()
	os.Exit(status)
}

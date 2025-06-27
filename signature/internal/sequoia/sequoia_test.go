//go:build containers_image_sequoia

package sequoia

import (
	"bytes"
	"os"
	"os/exec"
	"testing"

	"github.com/containers/image/v5/signature/internal/sequoia/testcli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func exportKey(dir string, fingerprint string) ([]byte, error) {
	cmd := exec.Command("sq", "--home", dir, "key", "export", "--cert", fingerprint)
	return cmd.Output()
}

func TestNewMechanismFromDirectory(t *testing.T) {
	if err := testcli.CheckCliVersion("1.3.0"); err != nil {
		t.Skipf("sq not usable: %v", err)
	}
	dir := t.TempDir()
	_, err := NewMechanismFromDirectory(dir)
	if err != nil {
		t.Fatalf("unable to initialize a mechanism: %v", err)
	}
	_, err = testcli.GenerateKey(dir, "foo@example.org")
	if err != nil {
		t.Fatalf("unable to generate key: %v", err)
	}
	_, err = NewMechanismFromDirectory(dir)
	if err != nil {
		t.Fatalf("unable to initialize a mechanism: %v", err)
	}
}

func TestNewEphemeralMechanism(t *testing.T) {
	if err := testcli.CheckCliVersion("1.3.0"); err != nil {
		t.Skipf("sq not usable: %v", err)
	}
	dir := t.TempDir()
	fingerprint, err := testcli.GenerateKey(dir, "foo@example.org")
	if err != nil {
		t.Fatalf("unable to generate key: %v", err)
	}
	output, err := testcli.ExportCert(dir, fingerprint)
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
	if err := testcli.CheckCliVersion("1.3.0"); err != nil {
		t.Skipf("sq not usable: %v", err)
	}
	dir := t.TempDir()
	fingerprint, err := testcli.GenerateKey(dir, "foo@example.org")
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

func TestImportSignVerify(t *testing.T) {
	if err := testcli.CheckCliVersion("1.3.0"); err != nil {
		t.Skipf("sq not usable: %v", err)
	}
	dir := t.TempDir()
	fingerprint, err := testcli.GenerateKey(dir, "foo@example.org")
	if err != nil {
		t.Fatalf("unable to generate key: %v", err)
	}
	output, err := exportKey(dir, fingerprint)
	if err != nil {
		t.Fatalf("unable to export key: %v", err)
	}
	newDir := t.TempDir()
	m, err := NewMechanismFromDirectory(newDir)
	if err != nil {
		t.Fatalf("unable to initialize a mechanism: %v", err)
	}
	keyIdentities, err := m.ImportKeys(output)
	if err != nil {
		t.Fatalf("unable to import key: %v", err)
	}
	if len(keyIdentities) != 1 || keyIdentities[0] != fingerprint {
		t.Fatalf("keyIdentity differ from the original: %v != %v",
			keyIdentities[0], fingerprint)
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

func TestImportSignVerifyEphemeral(t *testing.T) {
	if err := testcli.CheckCliVersion("1.3.0"); err != nil {
		t.Skipf("sq not usable: %v", err)
	}
	dir := t.TempDir()
	fingerprint, err := testcli.GenerateKey(dir, "foo@example.org")
	if err != nil {
		t.Fatalf("unable to generate key: %v", err)
	}
	output, err := exportKey(dir, fingerprint)
	if err != nil {
		t.Fatalf("unable to export key: %v", err)
	}
	m, err := NewEphemeralMechanism()
	if err != nil {
		t.Fatalf("unable to initialize a mechanism: %v", err)
	}
	keyIdentities, err := m.ImportKeys(output)
	if err != nil {
		t.Fatalf("unable to import key: %v", err)
	}
	if len(keyIdentities) != 1 || keyIdentities[0] != fingerprint {
		t.Fatalf("keyIdentity differ from the original: %v != %v",
			keyIdentities[0], fingerprint)
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
	if err := testcli.CheckCliVersion("1.3.0"); err != nil {
		t.Skipf("sq not usable: %v", err)
	}
	dir := t.TempDir()
	fingerprint, err := testcli.GenerateKey(dir, "foo@example.org")
	require.NoError(t, err)
	publicKey, err := testcli.ExportCert(dir, fingerprint)
	require.NoError(t, err)
	m1, err := NewMechanismFromDirectory(dir)
	require.NoError(t, err)

	input := []byte("Hello, world!")
	sig, err := m1.Sign(input, fingerprint)
	require.NoError(t, err)

	m2, err := NewEphemeralMechanism()
	require.NoError(t, err)

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

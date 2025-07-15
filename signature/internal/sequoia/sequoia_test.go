//go:build containers_image_sequoia

package sequoia

import (
	"bytes"
	"os"
	"testing"

	"github.com/containers/image/v5/signature/internal/sequoia/testcli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMechanismFromDirectory(t *testing.T) {
	if err := testcli.CheckCliVersion("1.3.0"); err != nil {
		t.Skipf("sq not usable: %v", err)
	}
	dir := t.TempDir()
	m, err := NewMechanismFromDirectory(dir)
	require.NoError(t, err)
	m.Close()
	_, err = testcli.GenerateKey(t, dir, "foo@example.org", "")
	if err != nil {
		t.Fatalf("unable to generate key: %v", err)
	}
	m, err = NewMechanismFromDirectory(dir)
	require.NoError(t, err)
	m.Close()

	t.Setenv("SEQUOIA_CRYPTO_POLICY", "this/does/not/exist") // Both unreadable files, and relative paths, should cause an error.
	_, err = NewMechanismFromDirectory(dir)
	assert.Error(t, err)
}

func TestNewEphemeralMechanism(t *testing.T) {
	if err := testcli.CheckCliVersion("1.3.0"); err != nil {
		t.Skipf("sq not usable: %v", err)
	}
	dir := t.TempDir()
	fingerprint, err := testcli.GenerateKey(t, dir, "foo@example.org", "")
	if err != nil {
		t.Fatalf("unable to generate key: %v", err)
	}
	output, err := testcli.ExportCert(dir, fingerprint)
	if err != nil {
		t.Fatalf("unable to export cert: %v", err)
	}
	m, err := NewEphemeralMechanism()
	require.NoError(t, err)
	defer m.Close()
	keyIdentities, err := m.ImportKeys(output)
	if err != nil {
		t.Fatalf("unable to import keys: %v", err)
	}
	if len(keyIdentities) != 1 || keyIdentities[0] != fingerprint {
		t.Fatalf("keyIdentity differ from the original: %v != %v",
			keyIdentities[0], fingerprint)
	}

	t.Setenv("SEQUOIA_CRYPTO_POLICY", "this/does/not/exist") // Both unreadable files, and relative paths, should cause an error.
	_, err = NewEphemeralMechanism()
	assert.Error(t, err)
}

func TestSignWithPassphrase(t *testing.T) {
	if err := testcli.CheckCliVersion("1.3.0"); err != nil {
		t.Skipf("sq not usable: %v", err)
	}

	// Success is tested in TestGenerateSignVerify and TestSignThenVerifyEphemeral

	// Invalid passphrase
	dir := t.TempDir()
	fingerprint, err := testcli.GenerateKey(t, dir, "foo@example.org", "valid-passphrase")
	require.NoError(t, err)
	m, err := NewMechanismFromDirectory(dir)
	require.NoError(t, err)
	defer m.Close()
	_, err = m.SignWithPassphrase([]byte("input"), fingerprint, "invalid-passphrase")
	assert.Error(t, err)
}

func TestGenerateSignVerify(t *testing.T) {
	if err := testcli.CheckCliVersion("1.3.0"); err != nil {
		t.Skipf("sq not usable: %v", err)
	}
	for _, passphrase := range []string{"", "test-passphrase"} {
		dir := t.TempDir()
		fingerprint, err := testcli.GenerateKey(t, dir, "foo@example.org", passphrase)
		if err != nil {
			t.Fatalf("unable to generate key: %v", err)
		}
		m, err := NewMechanismFromDirectory(dir)
		if err != nil {
			t.Fatalf("unable to initialize a mechanism: %v", err)
		}
		defer m.Close()
		input := []byte("Hello, world!")
		var sig []byte
		if passphrase != "" {
			sig, err = m.SignWithPassphrase(input, fingerprint, passphrase)
		} else {
			sig, err = m.Sign(input, fingerprint)
		}
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
}

func TestSignThenVerifyEphemeral(t *testing.T) {
	if err := testcli.CheckCliVersion("1.3.0"); err != nil {
		t.Skipf("sq not usable: %v", err)
	}
	dir := t.TempDir()
	fingerprint, err := testcli.GenerateKey(t, dir, "foo@example.org", "")
	require.NoError(t, err)
	publicKey, err := testcli.ExportCert(dir, fingerprint)
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

func TestImportKeys(t *testing.T) {
	// Success is tested in TestNewEphemeralMechanism and TestSignThenVerifyEphemeral
	m, err := NewEphemeralMechanism()
	require.NoError(t, err)
	defer m.Close()

	_, err = m.ImportKeys([]byte("This is not a key at all"))
	assert.Error(t, err)
}

func TestMain(m *testing.M) {
	err := Init()
	if err != nil {
		panic(err)
	}
	status := m.Run()
	os.Exit(status)
}

package signature

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testGPGHomeDirectory = "./fixtures"
)

func TestNewGPGSigningMechanism(t *testing.T) {
	// A dumb test just for code coverage. We test more with newGPGSigningMechanismInDirectory().
	_, err := NewGPGSigningMechanism()
	assert.NoError(t, err)
}

func TestNewGPGSigningMechanismInDirectory(t *testing.T) {
	// A dumb test just for code coverage.
	_, err := newGPGSigningMechanismInDirectory(testGPGHomeDirectory)
	assert.NoError(t, err)
	// The various GPG failure cases are not obviously easy to reach.
}

func TestGPGSigningMechanismImportKeysFromBytes(t *testing.T) {
	testDir, err := ioutil.TempDir("", "gpg-import-keys")
	require.NoError(t, err)
	defer os.RemoveAll(testDir)

	mech, err := newGPGSigningMechanismInDirectory(testDir)
	require.NoError(t, err)

	// Try validating a signature when the key is unknown.
	signature, err := ioutil.ReadFile("./fixtures/invalid-blob.signature")
	require.NoError(t, err)
	content, signingFingerprint, err := mech.Verify(signature)
	require.Error(t, err)

	// Successful import
	keyBlob, err := ioutil.ReadFile("./fixtures/public-key.gpg")
	require.NoError(t, err)
	keyIdentities, err := mech.ImportKeysFromBytes(keyBlob)
	require.NoError(t, err)
	assert.Equal(t, []string{TestKeyFingerprint}, keyIdentities)

	// After import, the signature should validate.
	content, signingFingerprint, err = mech.Verify(signature)
	require.NoError(t, err)
	assert.Equal(t, []byte("This is not JSON\n"), content)
	assert.Equal(t, TestKeyFingerprint, signingFingerprint)

	// Two keys: just concatenate the valid input twice.
	keyIdentities, err = mech.ImportKeysFromBytes(bytes.Join([][]byte{keyBlob, keyBlob}, nil))
	require.NoError(t, err)
	assert.Equal(t, []string{TestKeyFingerprint, TestKeyFingerprint}, keyIdentities)

	// Invalid input: This is accepted anyway by GPG, just returns no keys.
	keyIdentities, err = mech.ImportKeysFromBytes([]byte("This is invalid"))
	require.NoError(t, err)
	assert.Equal(t, []string{}, keyIdentities)
	// The various GPG/GPGME failures cases are not obviously easy to reach.
}

func TestGPGSigningMechanismSign(t *testing.T) {
	mech, err := newGPGSigningMechanismInDirectory(testGPGHomeDirectory)
	require.NoError(t, err)

	// Successful signing
	content := []byte("content")
	signature, err := mech.Sign(content, TestKeyFingerprint)
	require.NoError(t, err)

	signedContent, signingFingerprint, err := mech.Verify(signature)
	require.NoError(t, err)
	assert.EqualValues(t, content, signedContent)
	assert.Equal(t, TestKeyFingerprint, signingFingerprint)

	// Error signing
	_, err = mech.Sign(content, "this fingerprint doesn't exist")
	assert.Error(t, err)
	// The various GPG/GPGME failures cases are not obviously easy to reach.
}

func assertSigningError(t *testing.T, content []byte, fingerprint string, err error) {
	assert.Error(t, err)
	assert.Nil(t, content)
	assert.Empty(t, fingerprint)
}

func TestGPGSigningMechanismVerify(t *testing.T) {
	mech, err := newGPGSigningMechanismInDirectory(testGPGHomeDirectory)
	require.NoError(t, err)

	// Successful verification
	signature, err := ioutil.ReadFile("./fixtures/invalid-blob.signature")
	require.NoError(t, err)
	content, signingFingerprint, err := mech.Verify(signature)
	require.NoError(t, err)
	assert.Equal(t, []byte("This is not JSON\n"), content)
	assert.Equal(t, TestKeyFingerprint, signingFingerprint)

	// For extra paranoia, test that we return nil data on error.

	// Completely invalid signature.
	content, signingFingerprint, err = mech.Verify([]byte{})
	assertSigningError(t, content, signingFingerprint, err)

	content, signingFingerprint, err = mech.Verify([]byte("invalid signature"))
	assertSigningError(t, content, signingFingerprint, err)

	// Literal packet, not a signature
	signature, err = ioutil.ReadFile("./fixtures/unsigned-literal.signature")
	require.NoError(t, err)
	content, signingFingerprint, err = mech.Verify(signature)
	assertSigningError(t, content, signingFingerprint, err)

	// Encrypted data, not a signature.
	signature, err = ioutil.ReadFile("./fixtures/unsigned-encrypted.signature")
	require.NoError(t, err)
	content, signingFingerprint, err = mech.Verify(signature)
	assertSigningError(t, content, signingFingerprint, err)

	// FIXME? Is there a way to create a multi-signature so that gpgme_op_verify returns multiple signatures?

	// Expired signature
	signature, err = ioutil.ReadFile("./fixtures/expired.signature")
	require.NoError(t, err)
	content, signingFingerprint, err = mech.Verify(signature)
	assertSigningError(t, content, signingFingerprint, err)

	// Corrupt signature
	signature, err = ioutil.ReadFile("./fixtures/corrupt.signature")
	require.NoError(t, err)
	content, signingFingerprint, err = mech.Verify(signature)
	assertSigningError(t, content, signingFingerprint, err)

	// Valid signature with an unknown key
	signature, err = ioutil.ReadFile("./fixtures/unknown-key.signature")
	require.NoError(t, err)
	content, signingFingerprint, err = mech.Verify(signature)
	assertSigningError(t, content, signingFingerprint, err)

	// The various GPG/GPGME failures cases are not obviously easy to reach.
}

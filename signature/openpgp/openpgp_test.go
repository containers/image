package openpgp

import (
	"io/ioutil"
	"testing"

	"github.com/containers/image/signature"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImportKeysFromBytes(t *testing.T) {
	m, _ := NewOpenPGPSigningMechanism()
	keyBlob, err := ioutil.ReadFile("../fixtures/public-key.gpg")
	require.NoError(t, err)
	keyIdentities, err := m.ImportKeysFromBytes(keyBlob)
	require.NoError(t, err)
	assert.Equal(t, []string{signature.TestKeyFingerprint}, keyIdentities)
}

func TestVerify(t *testing.T) {

	type tc struct {
		title        string
		fixture      string
		fixtureBytes []byte
		isValid      bool
		identity     string
		publicKey    string
	}

	cases := []tc{
		{
			title:     "should verify valid signature using public key",
			fixture:   "image.signature",
			publicKey: "public-key.gpg",
			identity:  signature.TestKeyFingerprint,
			isValid:   true,
		},
		{
			title:   "should fail verifying valid signature with no public key",
			fixture: "image.signature",
		},
		{
			title:        "should fail verifying empty signature with valid public key",
			fixtureBytes: []byte{},
			publicKey:    "public-key.gpg",
		},
		{
			title:        "should fail verifying invalid signature with valid public key",
			fixtureBytes: []byte("invalid signature"),
			publicKey:    "public-key.gpg",
		},
		{
			title:     "should fail verifying unknown signature with valid public key",
			fixture:   "unknown-key.signature",
			publicKey: "public-key.gpg",
		},
		{
			title:     "should fail verifying unsigned but encrypted signature with valid public key",
			fixture:   "unsigned-encrypted.signature",
			publicKey: "public-key.gpg",
		},
		{
			title:     "should fail verifying unsigned literal signature with valid public key",
			fixture:   "unsigned-literal.signature",
			publicKey: "public-key.gpg",
		},
		{
			title:     "should fail verifying expired signature with valid public key",
			fixture:   "expired.signature",
			publicKey: "public-key.gpg",
		},
		{
			title:     "should fail verifying corrupted signature with valid public key",
			fixture:   "corrupt.signature",
			publicKey: "public-key.gpg",
		},
	}

	for _, c := range cases {
		t.Logf("it %s", c.title)
		m, _ := NewOpenPGPSigningMechanism()
		if len(c.publicKey) > 0 {
			key, err := ioutil.ReadFile("../fixtures/" + c.publicKey)
			require.NoError(t, err)
			_, err = m.ImportKeysFromBytes(key)
			require.NoError(t, err)
		}
		var s []byte
		if len(c.fixture) > 0 {
			var err error
			s, err = ioutil.ReadFile("../fixtures/" + c.fixture)
			require.NoError(t, err)
		}
		if len(c.fixtureBytes) > 0 {
			s = c.fixtureBytes
		}
		_, identity, err := m.Verify(s)
		if c.isValid {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
		if len(c.identity) > 0 {
			require.Equal(t, c.identity, identity)
		} else {
			require.Empty(t, identity)
		}
	}
}

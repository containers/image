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
		fixture      string
		fixtureBytes []byte
		isValid      bool
		identity     string
		publicKey    string
	}

	cases := []tc{
		{
			fixture:   "image.signature",
			publicKey: "public-key.gpg",
			identity:  signature.TestKeyFingerprint,
			isValid:   true,
		},
		{
			fixture: "image.signature",
		},
		{
			fixtureBytes: []byte{},
			publicKey:    "public-key.gpg",
		},
		{
			fixtureBytes: []byte("invalid signature"),
			publicKey:    "public-key.gpg",
		},
		{
			fixture:   "unknown-key.signature",
			publicKey: "public-key.gpg",
		},
		{
			fixture:   "unsigned-encrypted.signature",
			publicKey: "public-key.gpg",
		},
		{
			fixture:   "unsigned-literal.signature",
			publicKey: "public-key.gpg",
		},
		{
			fixture:   "expired.signature",
			publicKey: "public-key.gpg",
		},
		{
			fixture:   "corrupt.signature",
			publicKey: "public-key.gpg",
		},
	}

	for i, c := range cases {
		m, _ := NewOpenPGPSigningMechanism()
		if len(c.publicKey) > 0 {
			key, err := ioutil.ReadFile("../fixtures/" + c.publicKey)
			require.NoError(t, err)
			_, err = m.ImportKeysFromBytes(key)
			require.NoError(t, err)
		}
		var s []byte
		if len(c.fixture) > 0 {
			t.Logf("#%d case: %q", i, c.fixture)
			var err error
			s, err = ioutil.ReadFile("../fixtures/" + c.fixture)
			require.NoError(t, err)
		}
		if len(c.fixtureBytes) > 0 {
			t.Logf("#%d case: %q", i, c.fixtureBytes)
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

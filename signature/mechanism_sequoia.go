//go:build containers_image_sequoia

package signature

import (
	"os"
	"path"

	"github.com/containers/image/v5/signature/internal/sequoia"
	"github.com/containers/storage/pkg/homedir"
)

// A GPG/OpenPGP signing mechanism, implemented using Sequoia.
type sequoiaSigningMechanism struct {
	inner *sequoia.SigningMechanism
}

// newGPGSigningMechanismInDirectory returns a new GPG/OpenPGP signing mechanism, using optionalDir if not empty.
// The caller must call .Close() on the returned SigningMechanism.
func newGPGSigningMechanismInDirectory(optionalDir string) (signingMechanismWithPassphrase, error) {
	// For compatibility reasons, we allow both sequoiaHome and
	// gpgHome to be the same directory as designated by
	// optionalDir or GNUPGHOME.
	envHome := os.Getenv("GNUPGHOME")

	gpgHome := optionalDir
	if gpgHome == "" {
		gpgHome = envHome
		if gpgHome == "" {
			gpgHome = path.Join(homedir.Get(), ".gnupg")
		}
	}

	sequoiaHome := optionalDir
	if sequoiaHome == "" {
		sequoiaHome = envHome
		if sequoiaHome == "" {
			dataHome, err := homedir.GetDataHome()
			if err != nil {
				return nil, err
			}
			sequoiaHome = path.Join(dataHome, "sequoia")
		}
	}

	mech, err := sequoia.NewMechanismFromDirectory(sequoiaHome)
	if err != nil {
		return nil, err
	}

	// Migrate GnuPG keyrings if exist.
	for _, keyring := range []string{"pubring.gpg", "secring.gpg"} {
		blob, err := os.ReadFile(path.Join(gpgHome, keyring))
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, err
			}
		} else if _, err := mech.ImportKeys(blob); err != nil {
			return nil, err
		}
	}

	return &sequoiaSigningMechanism{
		inner: mech,
	}, nil
}

// newEphemeralGPGSigningMechanism returns a new GPG/OpenPGP signing mechanism which
// recognizes _only_ public keys from the supplied blobs, and returns the identities
// of these keys.
// The caller must call .Close() on the returned SigningMechanism.
func newEphemeralGPGSigningMechanism(blobs [][]byte) (signingMechanismWithPassphrase, []string, error) {
	mech, err := sequoia.NewEphemeralMechanism()
	if err != nil {
		return nil, nil, err
	}
	keyIdentities := []string{}
	for _, blob := range blobs {
		ki, err := mech.ImportKeys(blob)
		if err != nil {
			return nil, nil, err
		}
		keyIdentities = append(keyIdentities, ki...)
	}

	return &sequoiaSigningMechanism{
		inner: mech,
	}, keyIdentities, nil
}

func (m *sequoiaSigningMechanism) Close() error {
	return m.inner.Close()
}

// SupportsSigning returns nil if the mechanism supports signing, or a SigningNotSupportedError.
func (m *sequoiaSigningMechanism) SupportsSigning() error {
	return nil
}

// Sign creates a (non-detached) signature of input using keyIdentity and passphrase.
// Fails with a SigningNotSupportedError if the mechanism does not support signing.
func (m *sequoiaSigningMechanism) SignWithPassphrase(input []byte, keyIdentity string, passphrase string) ([]byte, error) {
	return m.inner.SignWithPassphrase(input, keyIdentity, passphrase)
}

// Sign creates a (non-detached) signature of input using keyIdentity.
// Fails with a SigningNotSupportedError if the mechanism does not support signing.
func (m *sequoiaSigningMechanism) Sign(input []byte, keyIdentity string) ([]byte, error) {
	return m.inner.Sign(input, keyIdentity)
}

// Verify parses unverifiedSignature and returns the content and the signer's identity.
// For mechanisms created using NewEphemeralGPGSigningMechanism, the returned key identity
// is expected to be one of the values returned by NewEphemeralGPGSigningMechanism,
// or the mechanism should implement signingMechanismWithVerificationIdentityLookup.
func (m *sequoiaSigningMechanism) Verify(unverifiedSignature []byte) (contents []byte, keyIdentity string, err error) {
	return m.inner.Verify(unverifiedSignature)
}

// UntrustedSignatureContents returns UNTRUSTED contents of the signature WITHOUT ANY VERIFICATION,
// along with a short identifier of the key used for signing.
// WARNING: The short key identifier (which corresponds to "Key ID" for OpenPGP keys)
// is NOT the same as a "key identity" used in other calls to this interface, and
// the values may have no recognizable relationship if the public key is not available.
func (m *sequoiaSigningMechanism) UntrustedSignatureContents(untrustedSignature []byte) (untrustedContents []byte, shortKeyIdentifier string, err error) {
	return gpgUntrustedSignatureContents(untrustedSignature)
}

func init() {
	err := sequoia.Init()
	if err != nil {
		panic("sequoia cannot be loaded: " + err.Error())
	}
}

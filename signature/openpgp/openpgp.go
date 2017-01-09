package openpgp

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/containers/image/types"

	"golang.org/x/crypto/openpgp"
)

type context struct {
	keyring openpgp.EntityList
}

// pgpSigningMechanism is a SignatureMechanism implementation using native Go
type pgpSigningMechanism struct {
	ctx *context
}

const armoredGPGPrefix = "-----BEGIN PGP PUBLIC KEY"

// NewOpenPGPSigningMechanism initializes the pgpSigningMechanism
func NewOpenPGPSigningMechanism() (types.SigningMechanism, error) {
	return pgpSigningMechanism{ctx: &context{
		keyring: openpgp.EntityList{},
	}}, nil
}

// ImportKeysFromBytes implements SigningMechanism.ImportKeysFromBytes
func (m pgpSigningMechanism) ImportKeysFromBytes(blob []byte) ([]string, error) {
	isArmored := strings.HasPrefix(string(blob), armoredGPGPrefix)
	var (
		keyring openpgp.EntityList
		err     error
	)
	if isArmored {
		keyring, err = openpgp.ReadArmoredKeyRing(bytes.NewReader(blob))
	} else {
		keyring, err = openpgp.ReadKeyRing(bytes.NewReader(blob))
	}
	if err != nil {
		return nil, err
	}
	keyIdentities := []string{}
	for _, entity := range keyring {
		if entity.PrimaryKey == nil {
			continue
		}
		m.ctx.keyring = append(m.ctx.keyring, entity)
		keyIdentities = append(keyIdentities, strings.ToUpper(fmt.Sprintf("%x", entity.PrimaryKey.Fingerprint)))
	}
	return keyIdentities, nil
}

// Sign implements SigningMechanism.Sign
func (m pgpSigningMechanism) Sign(input []byte, keyIdentity string) ([]byte, error) {
	return nil, errors.New("signing not implemented")
}

// Verify implements SigningMechanism.Verify
func (m pgpSigningMechanism) Verify(unverifiedSignature []byte) (contents []byte, keyIdentity string, err error) {
	md, err := openpgp.ReadMessage(bytes.NewReader(unverifiedSignature), m.ctx.keyring, nil, nil)
	if err != nil {
		return nil, "", err
	}
	content, err := ioutil.ReadAll(md.UnverifiedBody)
	if err != nil {
		return nil, "", err
	}
	if !md.IsSigned {
		return nil, "", errors.New("not signed")
	}
	if md.SignatureError != nil {
		return nil, "", fmt.Errorf("signature error: %v", md.SignatureError)
	}
	if md.SignedBy == nil {
		return nil, "", types.NewInvalidSignatureError("invalid GPG signature")
	}
	if md.Signature.SigLifetimeSecs != nil {
		expiry := md.Signature.CreationTime.Add(time.Duration(*md.Signature.SigLifetimeSecs) * time.Second)
		if time.Now().After(expiry) {
			return nil, "", fmt.Errorf("signature expired")
		}
	}

	// Uppercase the fingerprint to be compatible with gpgme
	return content, strings.ToUpper(fmt.Sprintf("%x", md.SignedBy.PublicKey.Fingerprint)), nil
}

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

type OpenPGPMechanism struct {
	ctx *context
}

func NewOpenPGPSigningMechanism() (types.SigningMechanism, error) {
	return OpenPGPMechanism{ctx: &context{
		keyring: openpgp.EntityList{},
	}}, nil
}

func (m OpenPGPMechanism) ImportKeysFromBytes(blob []byte) ([]string, error) {
	keyring, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(blob))
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

func (m OpenPGPMechanism) Sign(input []byte, keyIdentity string) ([]byte, error) {
	return nil, errors.New("signing not implemented")
}

func (m OpenPGPMechanism) Verify(unverifiedSignature []byte) (contents []byte, keyIdentity string, err error) {
	if len(m.ctx.keyring) == 0 {
		return nil, "", errors.New("no public keys imported")
	}
	md, err := openpgp.ReadMessage(bytes.NewReader(unverifiedSignature), m.ctx.keyring, nil, nil)
	if err != nil {
		return nil, "", err
	}
	if !md.IsSigned {
		return nil, "", errors.New("not signed")
	}
	content, err := ioutil.ReadAll(md.UnverifiedBody)
	if err != nil {
		return nil, "", err
	}
	if md.SignatureError != nil {
		return nil, "", fmt.Errorf("signature error: %v", md.SignatureError)
	}
	if md.SignedBy == nil {
		return nil, "", types.NewInvalidSignatureError(fmt.Sprintf("Invalid GPG signature: %#v", md.Signature))
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

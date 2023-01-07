package sigstore

import (
	"errors"
	"fmt"
	"os"

	internalSigner "github.com/containers/image/v5/internal/signer"
	"github.com/containers/image/v5/signature/signer"
	"github.com/containers/image/v5/signature/sigstore/internal"
)

type Option = internal.Option

func WithPrivateKeyFile(file string, passphrase []byte) Option {
	return func(s *internal.SigstoreSigner) error {
		if s.PrivateKey != nil {
			return fmt.Errorf("multiple private key sources specified when preparing to create sigstore signatures")
		}

		if passphrase == nil {
			return errors.New("private key passphrase not provided")
		}

		privateKeyPEM, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("reading private key from %s: %w", file, err)
		}
		signerVerifier, err := loadPrivateKey(privateKeyPEM, passphrase)
		if err != nil {
			return fmt.Errorf("initializing private key: %w", err)
		}
		s.PrivateKey = signerVerifier
		return nil
	}
}

func NewSigner(opts ...Option) (*signer.Signer, error) {
	s := internal.SigstoreSigner{}
	for _, o := range opts {
		if err := o(&s); err != nil {
			return nil, err
		}
	}
	if s.PrivateKey == nil {
		return nil, fmt.Errorf("preparing to create a sigstore signature: nothing to sign with provided")
	}

	return internalSigner.NewSigner(&s), nil
}

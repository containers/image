//go:build containers_image_sequoia

package simplesequoia

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/containers/image/v5/docker/reference"
	internalSig "github.com/containers/image/v5/internal/signature"
	internalSigner "github.com/containers/image/v5/internal/signer"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/signature/internal/sequoia"
	"github.com/containers/image/v5/signature/signer"
)

// simpleSequoiaSigner is a signer.SignerImplementation implementation for simple signing signatures using Sequoia.
type simpleSequoiaSigner struct {
	mech           *sequoia.SigningMechanism
	sequoiaHome    string // "" if using the system’s default
	keyFingerprint string
	passphrase     string // "" if not provided.
}

type Option func(*simpleSequoiaSigner) error

// WithSequoiaHome returns an Option for NewSigner, specifying a Sequoia home directory to use.
func WithSequoiaHome(sequoiaHome string) Option {
	return func(s *simpleSequoiaSigner) error {
		if sequoiaHome == "none" || sequoiaHome == "default" { // FIXME: Do we need to special-case this?
			return fmt.Errorf("unsupported special-case Sequoia-PGP home directory %q", sequoiaHome)
		}
		s.sequoiaHome = sequoiaHome
		return nil
	}
}

// WithKeyFingerprint returns an Option for NewSigner, specifying a key to sign with, using the provided GPG key fingerprint.
func WithKeyFingerprint(keyFingerprint string) Option {
	return func(s *simpleSequoiaSigner) error {
		s.keyFingerprint = keyFingerprint
		return nil
	}
}

// WithPassphrase returns an Option for NewSigner, specifying a passphrase for the private key.
func WithPassphrase(passphrase string) Option {
	return func(s *simpleSequoiaSigner) error {
		// The gpgme implementation can’t use passphrase with \n; reject it here for consistent behavior.
		// FIXME: We don’t need it in this API at all, but the "\n" check exists in the current call stack. That should go away.
		if strings.Contains(passphrase, "\n") {
			return errors.New("invalid passphrase: must not contain a line break")
		}
		s.passphrase = passphrase
		return nil
	}
}

// NewSigner returns a signature.Signer which creates “simple signing” signatures using the user’s default
// Sequoia PGP configuration.
//
// The set of options must identify a key to sign with, probably using a WithKeyFingerprint.
//
// The caller must call Close() on the returned Signer.
func NewSigner(opts ...Option) (*signer.Signer, error) {
	s := simpleSequoiaSigner{}
	for _, o := range opts {
		if err := o(&s); err != nil {
			return nil, err
		}
	}
	if s.sequoiaHome == "" {
		return nil, errors.New("FIXME: using the default Sequoia home is not currently implemented")
	}
	if s.keyFingerprint == "" {
		return nil, errors.New("no key identity provided for simple signing")
	}

	mech, err := sequoia.NewMechanismFromDirectory(s.sequoiaHome)
	if err != nil {
		return nil, fmt.Errorf("initializing Sequoia: %w", err)
	}
	s.mech = mech
	succeeded := false
	defer func() {
		if !succeeded {
			s.mech.Close()
		}
	}()
	if err := mech.SupportsSigning(); err != nil { // FIXME: Drop in internal/sequoia, move to mech_sequoia.go
		return nil, fmt.Errorf("Signing not supported: %w", err)
	}

	// Ideally, we should look up (and unlock?) the key at this point already. FIXME: is that possible? Anyway, low-priority.

	succeeded = true
	return internalSigner.NewSigner(&s), nil
}

// ProgressMessage returns a human-readable sentence that makes sense to write before starting to create a single signature.
func (s *simpleSequoiaSigner) ProgressMessage() string {
	return "Signing image using Sequoia-PGP simple signing"
}

// SignImageManifest creates a new signature for manifest m as dockerReference.
func (s *simpleSequoiaSigner) SignImageManifest(ctx context.Context, m []byte, dockerReference reference.Named) (internalSig.Signature, error) {
	if reference.IsNameOnly(dockerReference) {
		return nil, fmt.Errorf("reference %s can’t be signed, it has neither a tag nor a digest", dockerReference.String())
	}
	wrapped := sequoiaSigningOnlyMechanism{ // FIXME: Avoid this?
		inner: s.mech,
	}
	simpleSig, err := signature.SignDockerManifestWithOptions(m, dockerReference.String(), &wrapped, s.keyFingerprint, &signature.SignOptions{
		Passphrase: s.passphrase,
	})
	if err != nil {
		return nil, err
	}
	return internalSig.SimpleSigningFromBlob(simpleSig), nil
}

func (s *simpleSequoiaSigner) Close() error {
	return s.mech.Close()
}

// FIXME: this should only be called once per process
// func init() {
// 	err := sequoia.Init()
// 	if err != nil {
// 		panic("sequoia cannot be loaded: " + err.Error())
// 	}
// }

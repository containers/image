package copy

import (
	"fmt"

	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/transports"
	perrors "github.com/pkg/errors"
)

// createSignature creates a new signature of manifest using keyIdentity.
func (c *copier) createSignature(manifest []byte, keyIdentity string, passphrase string, identity reference.Named) ([]byte, error) {
	mech, err := signature.NewGPGSigningMechanism()
	if err != nil {
		return nil, perrors.Wrap(err, "initializing GPG")
	}
	defer mech.Close()
	if err := mech.SupportsSigning(); err != nil {
		return nil, perrors.Wrap(err, "Signing not supported")
	}

	if identity != nil {
		if reference.IsNameOnly(identity) {
			return nil, fmt.Errorf("Sign identity must be a fully specified reference %s", identity)
		}
	} else {
		identity = c.dest.Reference().DockerReference()
		if identity == nil {
			return nil, fmt.Errorf("Cannot determine canonical Docker reference for destination %s", transports.ImageName(c.dest.Reference()))
		}
	}

	c.Printf("Signing manifest\n")
	newSig, err := signature.SignDockerManifestWithOptions(manifest, identity.String(), mech, keyIdentity, &signature.SignOptions{Passphrase: passphrase})
	if err != nil {
		return nil, perrors.Wrap(err, "creating signature")
	}
	return newSig, nil
}

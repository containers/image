package copy

import (
	"context"
	"fmt"

	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/internal/private"
	internalsig "github.com/containers/image/v5/internal/signature"
	internalSigner "github.com/containers/image/v5/internal/signer"
	"github.com/containers/image/v5/signature/sigstore"
	"github.com/containers/image/v5/signature/simplesigning"
	"github.com/containers/image/v5/transports"
)

// sourceSignatures returns signatures from unparsedSource based on options,
// and verifies that they can be used (to avoid copying a large image when we
// can tell in advance that it would ultimately fail)
func (c *copier) sourceSignatures(ctx context.Context, unparsed private.UnparsedImage, options *Options,
	gettingSignaturesMessage, checkingDestMessage string) ([]internalsig.Signature, error) {
	var sigs []internalsig.Signature
	if options.RemoveSignatures {
		sigs = []internalsig.Signature{}
	} else {
		c.Printf("%s\n", gettingSignaturesMessage)
		s, err := unparsed.UntrustedSignatures(ctx)
		if err != nil {
			return nil, fmt.Errorf("reading signatures: %w", err)
		}
		sigs = s
	}
	if len(sigs) != 0 {
		c.Printf("%s\n", checkingDestMessage)
		if err := c.dest.SupportsSignatures(ctx); err != nil {
			return nil, fmt.Errorf("Can not copy signatures to %s: %w", transports.ImageName(c.dest.Reference()), err)
		}
	}
	return sigs, nil
}

// createSignature creates a new signature of manifest using keyIdentity.
func (c *copier) createSignature(ctx context.Context, manifest []byte, keyIdentity string, passphrase string, identity reference.Named) (internalsig.Signature, error) {
	opts := []simplesigning.Option{
		simplesigning.WithKeyFingerprint(keyIdentity),
	}
	if passphrase != "" {
		opts = append(opts, simplesigning.WithPassphrase(passphrase))
	}
	signer, err := simplesigning.NewSigner(opts...)
	if err != nil {
		return nil, err
	}
	defer signer.Close()

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

	c.Printf("%s\n", internalSigner.ProgressMessage(signer))
	newSig, err := internalSigner.SignImageManifest(ctx, signer, manifest, identity)
	if err != nil {
		return nil, fmt.Errorf("creating signature: %w", err)
	}
	return newSig, nil
}

// createSigstoreSignature creates a new sigstore signature of manifest using privateKeyFile and identity.
func (c *copier) createSigstoreSignature(ctx context.Context, manifest []byte, privateKeyFile string, passphrase []byte, identity reference.Named) (internalsig.Signature, error) {
	signer, err := sigstore.NewSigner(sigstore.WithPrivateKeyFile(privateKeyFile, passphrase))
	if err != nil {
		return nil, err
	}
	defer signer.Close()

	if identity != nil {
		if reference.IsNameOnly(identity) {
			return nil, fmt.Errorf("Sign identity must be a fully specified reference %s", identity.String())
		}
	} else {
		identity = c.dest.Reference().DockerReference()
		if identity == nil {
			return nil, fmt.Errorf("Cannot determine canonical Docker reference for destination %s", transports.ImageName(c.dest.Reference()))
		}
	}

	c.Printf("%s\n", internalSigner.ProgressMessage(signer))
	newSig, err := internalSigner.SignImageManifest(ctx, signer, manifest, identity)
	if err != nil {
		return nil, fmt.Errorf("creating signature: %w", err)
	}
	return newSig, nil
}

package copy

import (
	"fmt"

	man "github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/transports"
	"github.com/pkg/errors"
)

// createGPGSignature creates a new GPG signature of manifest using keyIdentity.
func (c *copier) createGPGSignature(manifest []byte, keyIdentity string) ([]byte, error) {
	mech, err := signature.NewGPGSigningMechanism()
	if err != nil {
		return nil, errors.Wrap(err, "initializing GPG")
	}
	defer mech.Close()
	if err := mech.SupportsSigning(); err != nil {
		return nil, errors.Wrap(err, "Signing not supported")
	}

	dockerReference := c.dest.Reference().DockerReference()
	if dockerReference == nil {
		return nil, errors.Errorf("Cannot determine canonical Docker reference for destination %s", transports.ImageName(c.dest.Reference()))
	}

	c.Printf("Signing manifest\n")
	newSig, err := signature.SignDockerManifest(manifest, dockerReference.String(), mech, keyIdentity)
	if err != nil {
		return nil, errors.Wrap(err, "creating signature")
	}
	return newSig, nil
}

// createSigstoreSignature creates a new signature of manifest.
func (c *copier) createSigstoreSignature(manifest []byte) ([]byte, error) {
	dockerReference := c.dest.Reference().DockerReference()
	if dockerReference == nil {
		return nil, fmt.Errorf("Cannot determine canonical Docker reference for destination %s", transports.ImageName(c.dest.Reference()))
	}

	manifestDigest, err := man.Digest(manifest)
	if err != nil {
		return nil, err
	}

	c.Printf("Signing manifest\n")
	newSignature, sigPayload, err := signature.SigstoreSignDockerManifest(c.sigstoreSign, manifestDigest, dockerReference.String())
	if err != nil {
		return nil, fmt.Errorf("creating signature: %w", err)
	}

	c.Printf("Uploading entry to transparency log\n")
	tlogEntry, err := signature.SigstoreUploadTransparencyLogEntry(c.sigstoreSign, newSignature, sigPayload)

	if err != nil {
		return nil, fmt.Errorf("error uploading entry to transparency log for %s: %w", dockerReference.String(), err)
	}
	c.Printf("Rekor entry successful. Index number: %d\n", tlogEntry)

	return newSignature, nil
}

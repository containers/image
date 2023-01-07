package sigstore

import (
	"fmt"
	"os"

	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/internal/signature"
	"github.com/containers/image/v5/signature/sigstore/internal"
)

// SignDockerManifestWithPrivateKeyFileUnstable returns a signature for manifest as the specified dockerReference,
// using a private key and an optional passphrase.
//
// Yes, this returns an internal type, and should currently not be used outside of c/image.
// There is NO COMITTMENT TO STABLE API.
func SignDockerManifestWithPrivateKeyFileUnstable(m []byte, dockerReference reference.Named, privateKeyFile string, passphrase []byte) (signature.Signature, error) {
	privateKeyPEM, err := os.ReadFile(privateKeyFile)
	if err != nil {
		return nil, fmt.Errorf("reading private key from %s: %w", privateKeyFile, err)
	}
	signerVerifier, err := loadPrivateKey(privateKeyPEM, passphrase)
	if err != nil {
		return nil, fmt.Errorf("initializing private key: %w", err)
	}

	signer := internal.SigstoreSigner{
		PrivateKey: signerVerifier,
	}
	return signer.SignImageManifest(m, dockerReference)
}

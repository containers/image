package cosign

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"

	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/internal/signature"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/signature/internal"
	sigstoreSignature "github.com/sigstore/sigstore/pkg/signature"
)

// SignDockerManifestWithPrivateKeyFileUnstable returns a signature for manifest as the specified dockerReference,
// using a private key and an optional passphrase.
//
// Yes, this returns an internal type, and should currently not be used outside of c/image.
// There is NO COMITTMENT TO STABLE API.
func SignDockerManifestWithPrivateKeyFileUnstable(m []byte, dockerReference reference.Named, privateKeyFile string, passphrase []byte) (signature.Cosign, error) {
	privateKeyPEM, err := os.ReadFile(privateKeyFile)
	if err != nil {
		return signature.Cosign{}, fmt.Errorf("reading private key from %s: %w", privateKeyFile, err)
	}
	signer, err := loadPrivateKey(privateKeyPEM, passphrase)
	if err != nil {
		return signature.Cosign{}, fmt.Errorf("initializing private key: %w", err)
	}

	return signDockerManifest(m, dockerReference, signer)
}

func signDockerManifest(m []byte, dockerReference reference.Named, signer sigstoreSignature.Signer) (signature.Cosign, error) {
	if reference.IsNameOnly(dockerReference) {
		return signature.Cosign{}, fmt.Errorf("reference %s can’t be signed, it has neither a tag nor a digest", dockerReference.String())
	}
	manifestDigest, err := manifest.Digest(m)
	if err != nil {
		return signature.Cosign{}, err
	}
	// AARGH. sigstore/cosign mostly ignores dockerReference for actual policy decisions.
	// That’s bad enough, BUT they also:
	// - Record the repo (but NOT THE TAG) in the value; without the tag we can’t detect version rollbacks.
	// - parse dockerReference @ dockerManifestDigest and expect that to be valid.
	// - It seems (FIXME: TEST THIS) that putting a repo:tag in would pass the current implementation.
	//   And signing digest references is _possible_ but probably rare (because signing typically happens on push, when
	//   the digest reference is not known in advance).
	//   SO: We put the full value in, which is not interoperable for signed digest references right now,
	//   and TODO: Talk sigstore/cosign to relax that.
	payloadData := internal.NewUntrustedCosignPayload(manifestDigest, dockerReference.String())
	payloadBytes, err := json.Marshal(payloadData)
	if err != nil {
		return signature.Cosign{}, err
	}

	// github.com/sigstore/cosign/internal/pkg/cosign.payloadSigner uses signatureoptions.WithContext(),
	// which seems to be not used by anything. So we don’t bother.
	signatureBytes, err := signer.SignMessage(bytes.NewReader(payloadBytes))
	if err != nil {
		return signature.Cosign{}, fmt.Errorf("creating signature: %w", err)
	}
	base64Signature := base64.StdEncoding.EncodeToString(signatureBytes)

	return signature.CosignFromComponents(signature.CosignSignatureMIMEType,
		payloadBytes,
		map[string]string{
			signature.CosignSignatureAnnotationKey: base64Signature,
		}), nil
}

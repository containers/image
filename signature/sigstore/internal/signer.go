package internal

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/internal/signature"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/signature/internal"
	sigstoreSignature "github.com/sigstore/sigstore/pkg/signature"
)

type SigstoreSigner struct {
	PrivateKey sigstoreSignature.Signer // May be nil during initialization
}

// ProgressMessage returns a human-readable sentence that makes sense to write before starting to create a single signature.
func (s *SigstoreSigner) ProgressMessage() string {
	return "Signing image using a sigstore signature"
}

// SignImageManifest creates a new signature for manifest m as dockerReference.
func (s *SigstoreSigner) SignImageManifest(ctx context.Context, m []byte, dockerReference reference.Named) (signature.Signature, error) {
	if reference.IsNameOnly(dockerReference) {
		return nil, fmt.Errorf("reference %s can’t be signed, it has neither a tag nor a digest", dockerReference.String())
	}
	manifestDigest, err := manifest.Digest(m)
	if err != nil {
		return nil, err
	}
	// sigstore/cosign completely ignores dockerReference for actual policy decisions.
	// They record the repo (but NOT THE TAG) in the value; without the tag we can’t detect version rollbacks.
	// So, just do what simple signing does, and cosign won’t mind.
	payloadData := internal.NewUntrustedSigstorePayload(manifestDigest, dockerReference.String())
	payloadBytes, err := json.Marshal(payloadData)
	if err != nil {
		return nil, err
	}

	// github.com/sigstore/cosign/internal/pkg/cosign.payloadSigner uses signatureoptions.WithContext(),
	// which seems to be not used by anything. So we don’t bother.
	signatureBytes, err := s.PrivateKey.SignMessage(bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("creating signature: %w", err)
	}
	base64Signature := base64.StdEncoding.EncodeToString(signatureBytes)

	return signature.SigstoreFromComponents(signature.SigstoreSignatureMIMEType,
		payloadBytes,
		map[string]string{
			signature.SigstoreSignatureAnnotationKey: base64Signature,
		}), nil
}

func (s *SigstoreSigner) Close() error {
	return nil
}

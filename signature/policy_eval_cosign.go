// Policy evaluation for prCosignSigned.

package signature

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/containers/image/v5/internal/private"
	"github.com/containers/image/v5/internal/signature"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/signature/internal"
	digest "github.com/opencontainers/go-digest"
	"github.com/sigstore/sigstore/pkg/cryptoutils"
)

func (pr *prCosignSigned) isSignatureAuthorAccepted(ctx context.Context, image private.UnparsedImage, sig []byte) (signatureAcceptanceResult, *Signature, error) {
	// We don’t know of a single user of this API, and we might return unexpected values in Signature.
	// For now, just punt.
	return sarRejected, nil, errors.New("isSignatureAuthorAccepted is not implemented for Cosign")
}

func (pr *prCosignSigned) isSignatureAccepted(ctx context.Context, image private.UnparsedImage, sig signature.Cosign) (signatureAcceptanceResult, error) {
	if pr.KeyPath != "" && pr.KeyData != nil {
		return sarRejected, errors.New(`Internal inconsistency: both "keyPath" and "keyData" specified`)
	}
	// FIXME: move this to per-context initialization
	var publicKeyPEM []byte
	if pr.KeyData != nil {
		publicKeyPEM = pr.KeyData
	} else {
		d, err := os.ReadFile(pr.KeyPath)
		if err != nil {
			return sarRejected, err
		}
		publicKeyPEM = d
	}

	publicKey, err := cryptoutils.UnmarshalPEMToPublicKey(publicKeyPEM)
	if err != nil {
		return sarRejected, fmt.Errorf("parsing public key: %w", err)
	}

	untrustedAnnotations := sig.UntrustedAnnotations()
	untrustedBase64Signature, ok := untrustedAnnotations[signature.CosignSignatureAnnotationKey]
	if !ok {
		return sarRejected, fmt.Errorf("missing %s annotation", signature.CosignSignatureAnnotationKey)
	}

	signature, err := internal.VerifyCosignPayload(publicKey, sig.UntrustedPayload(), untrustedBase64Signature, internal.CosignPayloadAcceptanceRules{
		ValidateSignedDockerReference: func(ref string) error {
			if !pr.SignedIdentity.matchesDockerReference(image, ref) {
				return PolicyRequirementError(fmt.Sprintf("Signature for identity %s is not accepted", ref))
			}
			return nil
		},
		ValidateSignedDockerManifestDigest: func(digest digest.Digest) error {
			m, _, err := image.Manifest(ctx)
			if err != nil {
				return err
			}
			digestMatches, err := manifest.MatchesDigest(m, digest)
			if err != nil {
				return err
			}
			if !digestMatches {
				return PolicyRequirementError(fmt.Sprintf("Signature for digest %s does not match", digest))
			}
			return nil
		},
	})
	if err != nil {
		return sarRejected, err
	}
	if signature == nil { // A paranoid sanity check that VerifyCosignPayload has returned consistent values
		return sarRejected, errors.New("internal error: VerifyCosignPayload succeeded but returned no data") // Coverage: This should never happen.
	}

	return sarAccepted, nil
}

func (pr *prCosignSigned) isRunningImageAllowed(ctx context.Context, image private.UnparsedImage) (bool, error) {
	sigs, err := image.UntrustedSignatures(ctx)
	if err != nil {
		return false, err
	}
	var rejections []error
	foundNonCosignSignatures := 0
	foundCosignNonAttachments := 0
	for _, s := range sigs {
		cosignSig, ok := s.(signature.Cosign)
		if !ok {
			foundNonCosignSignatures++
			continue
		}
		if cosignSig.UntrustedMIMEType() != signature.CosignSignatureMIMEType {
			foundCosignNonAttachments++
			continue
		}

		var reason error
		switch res, err := pr.isSignatureAccepted(ctx, image, cosignSig); res {
		case sarAccepted:
			// One accepted signature is enough.
			return true, nil
		case sarRejected:
			reason = err
		case sarUnknown:
			// Huh?! This should not happen at all; treat it as any other invalid value.
			fallthrough
		default:
			reason = fmt.Errorf(`Internal error: Unexpected signature verification result "%s"`, string(res))
		}
		rejections = append(rejections, reason)
	}
	var summary error
	switch len(rejections) {
	case 0:
		if foundNonCosignSignatures == 0 && foundCosignNonAttachments == 0 {
			// A nice message for the most common case.
			summary = PolicyRequirementError("A signature was required, but no signature exists")
		} else {
			summary = PolicyRequirementError(fmt.Sprintf("A signature was required, but no signature exists (%d non-Cosign signatures, %d Cosign non-signature attachments)",
				foundNonCosignSignatures, foundCosignNonAttachments))
		}
	case 1:
		summary = rejections[0]
	default:
		var msgs []string
		for _, e := range rejections {
			msgs = append(msgs, e.Error())
		}
		summary = PolicyRequirementError(fmt.Sprintf("None of the signatures were accepted, reasons: %s",
			strings.Join(msgs, "; ")))
	}
	return false, summary
}

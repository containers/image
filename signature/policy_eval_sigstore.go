// Policy evaluation for prSigstoreSigned.

package signature

import (
	"context"
	"crypto"
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

// loadBytesFromDataOrPath ensures there is at most one of ${prefix}Data and ${prefix}Path set,
// and returns the referenced data, or nil if neither is set.
func loadBytesFromDataOrPath(prefix string, data []byte, path string) ([]byte, error) {
	switch {
	case data != nil && path != "":
		return nil, fmt.Errorf(`Internal inconsistency: both "%sPath" and "%sData" specified`, prefix, prefix)
	case path != "":
		d, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return d, nil
	case data != nil:
		return data, nil
	default: // Nothing
		return nil, nil
	}
}

// sigstoreSignedTrustRoot contains an already parsed version of the prSigstoreSigned policy
type sigstoreSignedTrustRoot struct {
	publicKey crypto.PublicKey
}

func (pr *prSigstoreSigned) prepareTrustRoot() (*sigstoreSignedTrustRoot, error) {
	res := sigstoreSignedTrustRoot{}

	publicKeyPEM, err := loadBytesFromDataOrPath("key", pr.KeyData, pr.KeyPath)
	if err != nil {
		return nil, err
	}
	if publicKeyPEM != nil {
		pk, err := cryptoutils.UnmarshalPEMToPublicKey(publicKeyPEM)
		if err != nil {
			return nil, fmt.Errorf("parsing public key: %w", err)
		}
		res.publicKey = pk
	} else {
		return nil, errors.New("internal inconsistency: no public key specified")
	}

	return &res, nil
}

func (pr *prSigstoreSigned) isSignatureAuthorAccepted(ctx context.Context, image private.UnparsedImage, sig []byte) (signatureAcceptanceResult, *Signature, error) {
	// We don’t know of a single user of this API, and we might return unexpected values in Signature.
	// For now, just punt.
	return sarRejected, nil, errors.New("isSignatureAuthorAccepted is not implemented for sigstore")
}

func (pr *prSigstoreSigned) isSignatureAccepted(ctx context.Context, image private.UnparsedImage, sig signature.Sigstore) (signatureAcceptanceResult, error) {
	// FIXME: move this to per-context initialization
	trustRoot, err := pr.prepareTrustRoot()
	if err != nil {
		return sarRejected, err
	}

	untrustedAnnotations := sig.UntrustedAnnotations()
	untrustedBase64Signature, ok := untrustedAnnotations[signature.SigstoreSignatureAnnotationKey]
	if !ok {
		return sarRejected, fmt.Errorf("missing %s annotation", signature.SigstoreSignatureAnnotationKey)
	}
	untrustedPayload := sig.UntrustedPayload()

	publicKey := trustRoot.publicKey
	signature, err := internal.VerifySigstorePayload(publicKey, untrustedPayload, untrustedBase64Signature, internal.SigstorePayloadAcceptanceRules{
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
	if signature == nil { // A paranoid sanity check that VerifySigstorePayload has returned consistent values
		return sarRejected, errors.New("internal error: VerifySigstorePayload succeeded but returned no data") // Coverage: This should never happen.
	}

	return sarAccepted, nil
}

func (pr *prSigstoreSigned) isRunningImageAllowed(ctx context.Context, image private.UnparsedImage) (bool, error) {
	sigs, err := image.UntrustedSignatures(ctx)
	if err != nil {
		return false, err
	}
	var rejections []error
	foundNonSigstoreSignatures := 0
	foundSigstoreNonAttachments := 0
	for _, s := range sigs {
		sigstoreSig, ok := s.(signature.Sigstore)
		if !ok {
			foundNonSigstoreSignatures++
			continue
		}
		if sigstoreSig.UntrustedMIMEType() != signature.SigstoreSignatureMIMEType {
			foundSigstoreNonAttachments++
			continue
		}

		var reason error
		switch res, err := pr.isSignatureAccepted(ctx, image, sigstoreSig); res {
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
		if foundNonSigstoreSignatures == 0 && foundSigstoreNonAttachments == 0 {
			// A nice message for the most common case.
			summary = PolicyRequirementError("A signature was required, but no signature exists")
		} else {
			summary = PolicyRequirementError(fmt.Sprintf("A signature was required, but no signature exists (%d non-sigstore signatures, %d sigstore non-signature attachments)",
				foundNonSigstoreSignatures, foundSigstoreNonAttachments))
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

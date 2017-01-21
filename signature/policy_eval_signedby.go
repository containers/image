// Policy evaluation for prSignedBy.

package signature

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/pkg/errors"

	"github.com/containers/image/manifest"
	"github.com/containers/image/types"
	"github.com/opencontainers/go-digest"
)

func (pr *prSignedBy) validateKeyType() error {
	// use a pointer here so we can get nil
	switch pr.KeyType {
	case SBKeyTypeGPGKeys:
	case SBKeyTypeSignedByGPGKeys, SBKeyTypeX509Certificates, SBKeyTypeSignedByX509CAs:
		// FIXME? Reject this at policy parsing time already?
		return errors.Errorf(`"Unimplemented "keyType" value "%s"`, string(pr.KeyType))
	default:
		// This should never happen, newPRSignedBy ensures KeyType.IsValid()
		return errors.Errorf(`"Unknown "keyType" value "%s"`, string(pr.KeyType))
	}

	if pr.KeyPath != "" && pr.KeyData != nil {
		return errors.New(`Internal inconsistency: both "keyPath" and "keyData" specified`)
	}

	return nil
}

func (pr *prSignedBy) readKey() ([]byte, error) {
	// FIXME: move this to per-context initialization
	var data []byte
	var err error

	if pr.KeyData != nil {
		data = pr.KeyData
	} else {
		data, err = ioutil.ReadFile(pr.KeyPath)
		if err != nil {
			return nil, err
		}
	}

	return data, nil
}

func (pr *prSignedBy) isSignatureAuthorAccepted(image types.UnparsedImage, sig []byte) (signatureAcceptanceResult, *Signature, error) {
	if err := pr.validateKeyType(); err != nil {
		return sarRejected, nil, err
	}

	data, err := pr.readKey()
	if err != nil {
		return sarRejected, nil, err
	}

	// FIXME: move this to per-context initialization
	dir, err := ioutil.TempDir("", "skopeo-signedBy-")
	if err != nil {
		return sarRejected, nil, err
	}
	defer os.RemoveAll(dir)
	mech, err := newGPGSigningMechanismInDirectory(dir)
	if err != nil {
		return sarRejected, nil, err
	}

	trustedIdentities, err := mech.ImportKeysFromBytes(data)
	if err != nil {
		return sarRejected, nil, err
	}
	if len(trustedIdentities) == 0 {
		return sarRejected, nil, PolicyRequirementError("No public keys imported")
	}

	signature, err := verifyAndExtractSignature(mech, sig, signatureAcceptanceRules{
		validateKeyIdentity: func(keyIdentity string) error {
			for _, trustedIdentity := range trustedIdentities {
				if keyIdentity == trustedIdentity {
					return nil
				}
			}
			// Coverage: We use a private GPG home directory and only import trusted keys, so this should
			// not be reachable.
			return PolicyRequirementError(fmt.Sprintf("Signature by key %s is not accepted", keyIdentity))
		},
		validateSignedDockerReference: func(ref string) error {
			if !pr.SignedIdentity.matchesDockerReference(image, ref) {
				return PolicyRequirementError(fmt.Sprintf("Signature for identity %s is not accepted", ref))
			}
			return nil
		},
		validateSignedDockerManifestDigest: func(digest digest.Digest) error {
			m, _, err := image.Manifest()
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
		return sarRejected, nil, err
	}

	return sarAccepted, signature, nil
}

func (pr *prSignedBy) isRunningImageAllowed(image types.UnparsedImage) (bool, error) {
	sigs, err := image.Signatures()
	if err != nil {
		return false, err
	}
	var rejections []error
	for _, s := range sigs {
		var reason error
		switch res, _, err := pr.isSignatureAuthorAccepted(image, s); res {
		case sarAccepted:
			// One accepted signature is enough.
			return true, nil
		case sarRejected:
			reason = err
		case sarUnknown:
			// Huh?! This should not happen at all; treat it as any other invalid value.
			fallthrough
		default:
			reason = errors.Errorf(`Internal error: Unexpected signature verification result "%s"`, string(res))
		}
		rejections = append(rejections, reason)
	}
	var summary error
	switch len(rejections) {
	case 0:
		summary = PolicyRequirementError("A signature was required, but no signature exists")
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

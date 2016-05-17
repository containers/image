// Note: Consider the API unstable until the code supports at least three different image formats or transports.

package signature

import (
	"fmt"

	"github.com/projectatomic/skopeo/docker/utils"
)

// SignDockerManifest returns a signature for manifest as the specified dockerReference,
// using mech and keyIdentity.
func SignDockerManifest(manifest []byte, dockerReference string, mech SigningMechanism, keyIdentity string) ([]byte, error) {
	manifestDigest, err := utils.ManifestDigest(manifest)
	if err != nil {
		return nil, err
	}
	sig := privateSignature{
		Signature{
			DockerManifestDigest: manifestDigest,
			DockerReference:      dockerReference,
		},
	}
	return sig.sign(mech, keyIdentity)
}

// VerifyDockerManifestSignature checks that unverifiedSignature uses expectedKeyIdentity to sign unverifiedManifest as expectedDockerReference,
// using mech.
func VerifyDockerManifestSignature(unverifiedSignature, unverifiedManifest []byte,
	expectedDockerReference string, mech SigningMechanism, expectedKeyIdentity string) (*Signature, error) {
	expectedManifestDigest, err := utils.ManifestDigest(unverifiedManifest)
	if err != nil {
		return nil, err
	}
	sig, err := verifyAndExtractSignature(mech, unverifiedSignature, expectedKeyIdentity, expectedDockerReference)
	if err != nil {
		return nil, err
	}
	if sig.DockerManifestDigest != expectedManifestDigest {
		return nil, InvalidSignatureError{msg: fmt.Sprintf("Docker manifest digest %s does not match %s", sig.DockerManifestDigest, expectedManifestDigest)}
	}
	return sig, nil
}

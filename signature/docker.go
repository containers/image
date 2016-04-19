// Note: Consider the API unstable until the code supports at least three different image formats or transports.

package signature

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/docker/libtrust"
)

// A string representing a Docker manifest MIME type
type manifestMIMEType string

const (
	dockerV2Schema1MIMEType manifestMIMEType = "application/vnd.docker.distribution.manifest.v1+json"
	dockerV2Schema2MIMEType manifestMIMEType = "application/vnd.docker.distribution.manifest.v2+json"
)

// guessManifestMIMEType guesses MIME type of a manifest and returns it _if it is recognized_, or "" if unknown or unrecognized.
// FIXME? This should ideally use out-of-band MIME type instead of blindly parsing the manifest,
// but we may not have such metadata available (e.g. when the manifest is a local file).
func guessManifestMIMEType(manifest []byte) manifestMIMEType {
	// A subset of manifest fields; the rest is silently ignored by json.Unmarshal.
	// Also docker/distribution/manifest.Versioned.
	meta := struct {
		MediaType     string `json:"mediaType"`
		SchemaVersion int    `json:"schemaVersion"`
	}{}
	if err := json.Unmarshal(manifest, &meta); err != nil {
		return ""
	}

	switch meta.MediaType {
	case string(dockerV2Schema2MIMEType): // A recognized type.
		return manifestMIMEType(meta.MediaType)
	}
	switch meta.SchemaVersion {
	case 1:
		return dockerV2Schema1MIMEType
	case 2: // Really should not happen, meta.MediaType should have been set. But given the data, this is our best guess.
		return dockerV2Schema2MIMEType
	}
	return ""
}

func dockerManifestDigest(manifest []byte) (string, error) {
	if guessManifestMIMEType(manifest) == dockerV2Schema1MIMEType {
		sig, err := libtrust.ParsePrettySignature(manifest, "signatures")
		if err != nil {
			return "", err
		}
		manifest, err = sig.Payload()
		if err != nil {
			// Coverage: This should never happen, libtrust's Payload() can fail only if joseBase64UrlDecode() fails, on a string
			// that libtrust itself has josebase64UrlEncode()d
			return "", err
		}
	}

	hash := sha256.Sum256(manifest)
	return "sha256:" + hex.EncodeToString(hash[:]), nil
}

// SignDockerManifest returns a signature for manifest as the specified dockerReference,
// using mech and keyIdentity.
func SignDockerManifest(manifest []byte, dockerReference string, mech SigningMechanism, keyIdentity string) ([]byte, error) {
	manifestDigest, err := dockerManifestDigest(manifest)
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
	expectedManifestDigest, err := dockerManifestDigest(unverifiedManifest)
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

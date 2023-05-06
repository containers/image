//go:build containers_image_disable_signing
// +build containers_image_disable_signing

package signature

import "errors"

// errSigningDisabled is returned if container image signing is disabled.
var errSigningDisabled = errors.New("container image signing is disabled in this build")

// newGPGSigningMechanismInDirectory returns an error indicating signing is disabled.
func newGPGSigningMechanismInDirectory(optionalDir string) (SigningMechanism, error) {
	return nil, errSigningDisabled
}

// newEphemeralGPGSigningMechanism returns an error indicating signing is disabled.
func newEphemeralGPGSigningMechanism(blobs [][]byte) (SigningMechanism, []string, error) {
	return nil, nil, errSigningDisabled
}

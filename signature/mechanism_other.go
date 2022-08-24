//go:build !containers_image_openpgp && !containers_image_disable_signing && !cgo
// +build !containers_image_openpgp,!containers_image_disable_signing,!cgo

package signature

// CgoIsDisabled indicates as a compiler error that this package requires cgo.
//
// The containers_image_openpgp build tag enables the OpenPGP implementation but
// beware that it is not actively maintained and is considered insecure.
//
// The containers_image_disable_signing build tag alternatively disables signing
// and will return an error when attempting to sign an image at runtime.
//
// # github.com/containers/image/v5/signature
// ./mechanism_other.go:16:21: undefined: ContainersImageSignatureRequiresCgo
var CgoIsDisabled = ContainersImageSignatureRequiresCgo

// newGPGSigningMechanismInDirectory cannot be compiled without Cgo.
func newGPGSigningMechanismInDirectory(optionalDir string) (SigningMechanism, error) {
	return nil, CgoIsDisabled
}

// newEphemeralGPGSigningMechanism cannot be compiled without Cgo.
func newEphemeralGPGSigningMechanism(blobs [][]byte) (SigningMechanism, []string, error) {
	return nil, nil, CgoIsDisabled
}

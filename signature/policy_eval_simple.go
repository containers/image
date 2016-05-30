// Policy evaluation for the various simple PolicyRequirement types.

package signature

import "github.com/projectatomic/skopeo/types"

func (pr *prInsecureAcceptAnything) isSignatureAuthorAccepted(image types.Image, sig []byte) (signatureAcceptanceResult, *Signature, error) {
	// prInsecureAcceptAnything semantics: Every image is allowed to run,
	// but this does not consider the signature as verified.
	return sarUnknown, nil, nil
}

func (pr *prInsecureAcceptAnything) isRunningImageAllowed(image types.Image) (bool, error) {
	return true, nil
}

func (pr *prReject) isSignatureAuthorAccepted(image types.Image, sig []byte) (signatureAcceptanceResult, *Signature, error) {
	// FIXME? Name the image, or better the matched scope in Policy.Specific.
	return sarRejected, nil, PolicyRequirementError("Any signatures for these images are rejected by policy.")
}

func (pr *prReject) isRunningImageAllowed(image types.Image) (bool, error) {
	// FIXME? Name the image, or better the matched scope in Policy.Specific.
	return false, PolicyRequirementError("Running these images is rejected by policy.")
}

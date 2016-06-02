package signature

import "testing"

func TestPRInsecureAcceptAnythingIsSignatureAuthorAccepted(t *testing.T) {
	pr := NewPRInsecureAcceptAnything()
	// Pass nil pointers to, kind of, test that the return value does not depend on the parameters.
	sar, parsedSig, err := pr.isSignatureAuthorAccepted(nil, nil)
	assertSARUnknown(t, sar, parsedSig, err)
}

func TestPRInsecureAcceptAnythingIsRunningImageAllowed(t *testing.T) {
	pr := NewPRInsecureAcceptAnything()
	// Pass a nil pointer to, kind of, test that the return value does not depend on the image.
	res, err := pr.isRunningImageAllowed(nil)
	assertRunningAllowed(t, res, err)
}

func TestPRRejectIsSignatureAuthorAccepted(t *testing.T) {
	pr := NewPRReject()
	// Pass nil pointers to, kind of, test that the return value does not depend on the parameters.
	sar, parsedSig, err := pr.isSignatureAuthorAccepted(nil, nil)
	assertSARRejectedPolicyRequirement(t, sar, parsedSig, err)
}

func TestPRRejectIsRunningImageAllowed(t *testing.T) {
	// This will obviously need to change after this is implemented.
	pr := NewPRReject()
	// Pass a nil pointer to, kind of, test that the return value does not depend on the image.
	res, err := pr.isRunningImageAllowed(nil)
	assertRunningRejectedPolicyRequirement(t, res, err)
}

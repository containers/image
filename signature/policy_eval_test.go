package signature

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Helpers for validating PolicyRequirement.isSignatureAuthorAccepted results:

// assertSARRejected verifies that isSignatureAuthorAccepted returns a consistent sarRejected result
// with the expected signature.
func assertSARAccepted(t *testing.T, sar signatureAcceptanceResult, parsedSig *Signature, err error, expectedSig Signature) {
	assert.Equal(t, sarAccepted, sar)
	assert.Equal(t, &expectedSig, parsedSig)
	assert.NoError(t, err)
}

// assertSARRejected verifies that isSignatureAuthorAccepted returns a consistent sarRejected result.
func assertSARRejected(t *testing.T, sar signatureAcceptanceResult, parsedSig *Signature, err error) {
	assert.Equal(t, sarRejected, sar)
	assert.Nil(t, parsedSig)
	assert.Error(t, err)
}

// assertSARRejectedPolicyRequiremnt verifies that isSignatureAuthorAccepted returns a consistent sarRejected resul,
// and that the returned error is a PolicyRequirementError..
func assertSARRejectedPolicyRequirement(t *testing.T, sar signatureAcceptanceResult, parsedSig *Signature, err error) {
	assertSARRejected(t, sar, parsedSig, err)
	assert.IsType(t, PolicyRequirementError(""), err)
}

// assertSARRejected verifies that isSignatureAuthorAccepted returns a consistent sarUnknown result.
func assertSARUnknown(t *testing.T, sar signatureAcceptanceResult, parsedSig *Signature, err error) {
	assert.Equal(t, sarUnknown, sar)
	assert.Nil(t, parsedSig)
	assert.NoError(t, err)
}

// Helpers for validating PolicyRequirement.isRunningImageAllowed results:

// assertRunningAllowed verifies that isRunningImageAllowed returns a consistent true result
func assertRunningAllowed(t *testing.T, allowed bool, err error) {
	assert.Equal(t, true, allowed)
	assert.NoError(t, err)
}

// assertRunningRejected verifies that isRunningImageAllowed returns a consistent false result
func assertRunningRejected(t *testing.T, allowed bool, err error) {
	assert.Equal(t, false, allowed)
	assert.Error(t, err)
}

// assertRunningRejectedPolicyRequirement verifies that isRunningImageAllowed returns a consistent false result
// and that the returned error is a PolicyRequirementError.
func assertRunningRejectedPolicyRequirement(t *testing.T, allowed bool, err error) {
	assertRunningRejected(t, allowed, err)
	assert.IsType(t, PolicyRequirementError(""), err)
}

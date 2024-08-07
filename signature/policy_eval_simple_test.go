package signature

import (
	"context"
	"testing"

	"github.com/containers/image/v5/internal/testing/mocks"
	"github.com/containers/image/v5/types"
)

// nameOnlyImageMock is a mock of private.UnparsedImage which only allows transports.ImageName to work
type nameOnlyImageMock struct {
	mocks.ForbiddenUnparsedImage
}

func (nameOnlyImageMock) Reference() types.ImageReference {
	return nameOnlyImageReferenceMock{s: "== StringWithinTransport mock"}
}

// nameOnlyImageReferenceMock is a mock of types.ImageReference which only allows transports.ImageName to work, returning self.
type nameOnlyImageReferenceMock struct {
	mocks.ForbiddenImageReference
	s string
}

func (ref nameOnlyImageReferenceMock) Transport() types.ImageTransport {
	return mocks.NameImageTransport("== Transport mock")
}

func (ref nameOnlyImageReferenceMock) StringWithinTransport() string {
	return ref.s
}

func TestPRInsecureAcceptAnythingIsSignatureAuthorAccepted(t *testing.T) {
	pr := NewPRInsecureAcceptAnything()
	// Pass nil signature to, kind of, test that the return value does not depend on it.
	sar, parsedSig, err := pr.isSignatureAuthorAccepted(context.Background(), nameOnlyImageMock{}, nil)
	assertSARUnknown(t, sar, parsedSig, err)
}

func TestPRInsecureAcceptAnythingIsRunningImageAllowed(t *testing.T) {
	pr := NewPRInsecureAcceptAnything()
	res, err := pr.isRunningImageAllowed(context.Background(), nameOnlyImageMock{})
	assertRunningAllowed(t, res, err)
}

func TestPRRejectIsSignatureAuthorAccepted(t *testing.T) {
	pr := NewPRReject()
	// Pass nil signature to, kind of, test that the return value does not depend on it.
	sar, parsedSig, err := pr.isSignatureAuthorAccepted(context.Background(), nameOnlyImageMock{}, nil)
	assertSARRejectedPolicyRequirement(t, sar, parsedSig, err)
}

func TestPRRejectIsRunningImageAllowed(t *testing.T) {
	pr := NewPRReject()
	res, err := pr.isRunningImageAllowed(context.Background(), nameOnlyImageMock{})
	assertRunningRejectedPolicyRequirement(t, res, err)
}

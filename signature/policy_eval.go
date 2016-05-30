package signature

import "github.com/projectatomic/skopeo/types"

// PolicyRequirementError is an explanatory text for rejecting a signature or an image.
type PolicyRequirementError string

func (err PolicyRequirementError) Error() string {
	return string(err)
}

// signatureAcceptanceResult is the principal value returned by isSignatureAuthorAccepted.
type signatureAcceptanceResult string

const (
	sarAccepted signatureAcceptanceResult = "sarAccepted"
	sarRejected signatureAcceptanceResult = "sarRejected"
	sarUnknown  signatureAcceptanceResult = "sarUnknown"
)

// PolicyRequirement is a rule which must be satisfied by at least one of the signatures of an image.
// The type is public, but its definition is private.
type PolicyRequirement interface {
	// FIXME: For speed, we should support creating per-context state (not stored in the PolicyRequirement), to cache
	// costly initialization like creating temporary GPG home directories and reading files.
	// Setup() (someState, error)
	// Then, the operations below would be done on the someState object, not directly on a PolicyRequirement.

	// isSignatureAuthorAccepted, given an image and a signature blob, returns:
	// - sarAccepted if the signature has been verified against the appropriate public key
	//   (where "appropriate public key" may depend on the contents of the signature);
	//   in that case a parsed Signature should be returned.
	// - sarRejected if the signature has not been verified;
	//   in that case error must be non-nil, and should be an PolicyRequirementError if evaluation
	//   succeeded but the result was rejection.
	// - sarUnknown if if this PolicyRequirement does not deal with signatures.
	//   NOTE: sarUnknown should not be returned if this PolicyRequirement should make a decision but something failed.
	//   Returning sarUnknown and a non-nil error value is invalid.
	// WARNING: This makes the signature contents acceptable for futher processing,
	// but it does not necessarily mean that the contents of the signature are
	// consistent with local policy.
	// For example:
	// - Do not use a true value to determine whether to run
	//   a container based on this image; use IsRunningImageAllowed instead.
	// - Just because a signature is accepted does not automatically mean the contents of the
	//   signature are authorized to run code as root, or to affect system or cluster configuration.
	isSignatureAuthorAccepted(image types.Image, sig []byte) (signatureAcceptanceResult, *Signature, error)

	// isRunningImageAllowed returns true if the requirement allows running an image.
	// If it returns false, err must be non-nil, and should be an PolicyRequirementError if evaluation
	// succeeded but the result was rejection.
	isRunningImageAllowed(image types.Image) (bool, error)
}

// PolicyReferenceMatch specifies a set of image identities accepted in PolicyRequirement.
// The type is public, but its implementation is private.
type PolicyReferenceMatch interface {
	// matchesDockerReference decides whether a specific image identity is accepted for an image
	// (or, usually, for the image's IntendedDockerReference()),
	matchesDockerReference(image types.Image, signatureDockerReference string) bool
}

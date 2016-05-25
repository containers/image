package signature

import "github.com/projectatomic/skopeo/types"

// PolicyReferenceMatch specifies a set of image identities accepted in PolicyRequirement.
// The type is public, but its implementation is private.
type PolicyReferenceMatch interface {
	// matchesDockerReference decides whether a specific image identity is accepted for an image
	// (or, usually, for the image's IntendedDockerReference()),
	matchesDockerReference(image types.Image, signatureDockerReference string) bool
}

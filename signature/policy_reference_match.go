// PolicyReferenceMatch implementations.

package signature

import (
	"github.com/containers/image/docker/reference"
	"github.com/containers/image/types"
)

// parseDockerReferences converts two reference strings into parsed entities, failing on any error
func parseDockerReferences(s1, s2 string) (reference.Named, reference.Named, error) {
	r1, err := reference.ParseNamed(s1)
	if err != nil {
		return nil, nil, err
	}
	r2, err := reference.ParseNamed(s2)
	if err != nil {
		return nil, nil, err
	}
	return r1, r2, nil
}

func (prm *prmMatchExact) matchesDockerReference(image types.Image, signatureDockerReference string) bool {
	intended, signature, err := parseDockerReferences(image.IntendedDockerReference(), signatureDockerReference)
	if err != nil {
		return false
	}
	// Do not add default tags: image.IntendedDockerReference() has it added already per its construction, and signatureDockerReference should be exact; so, verify that now.
	if reference.IsNameOnly(intended) || reference.IsNameOnly(signature) {
		return false
	}
	return signature.String() == intended.String()
}

func (prm *prmMatchRepository) matchesDockerReference(image types.Image, signatureDockerReference string) bool {
	intended, signature, err := parseDockerReferences(image.IntendedDockerReference(), signatureDockerReference)
	if err != nil {
		return false
	}
	return signature.Name() == intended.Name()
}

func (prm *prmExactReference) matchesDockerReference(image types.Image, signatureDockerReference string) bool {
	intended, signature, err := parseDockerReferences(prm.DockerReference, signatureDockerReference)
	if err != nil {
		return false
	}
	// prm.DockerReference and signatureDockerReference should be exact; so, verify that now.
	if reference.IsNameOnly(intended) || reference.IsNameOnly(signature) {
		return false
	}
	return signature.String() == intended.String()
}

func (prm *prmExactRepository) matchesDockerReference(image types.Image, signatureDockerReference string) bool {
	intended, signature, err := parseDockerReferences(prm.DockerRepository, signatureDockerReference)
	if err != nil {
		return false
	}
	return signature.Name() == intended.Name()
}

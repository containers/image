package manifest

import (
	"fmt"
	"sort"
	"strings"

	"github.com/containers/image/types"
	digest "github.com/opencontainers/go-digest"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// List is an interface for parsing, modifying lists of image manifests.
// Callers can either use this abstract interface without understanding the
// details of the formats, or instantiate a specific implementation (e.g.
// manifest.OCI1Index) and access the public members directly.
type List interface {
	// MIMEType returns the MIME type of this particular manifest list.
	MIMEType() string

	// Instances returns a list of the manifests that this list knows of.
	Instances() []types.BlobInfo

	// Update information about the list's instances.  The length of the passed-in slice must
	// match the length of the list of instances which the list already contains, and every field
	// must be specified.
	UpdateInstances([]ListUpdate) error

	// ChooseDigest selects which manifest is most appropriate for the platform described by the
	// SystemContext, or for the current platform if the SystemContext doesn't specify any details.
	ChooseDigest(ctx *types.SystemContext) (digest.Digest, error)

	// ImageID computes a recommended image ID based on the list of images referred to by the manifest.
	ImageID() string

	// Serialize returns the list in a blob format.
	// NOTE: Serialize() does not in general reproduce the original blob if this object was loaded
	// from, even if no modifications were made!
	Serialize() ([]byte, error)

	// ToOCI1Index() returns the list rebuilt as an OCI1 index, converting it if necessary.
	ToOCI1Index() (*OCI1Index, error)

	// ToSchema2List() returns the list rebuilt as a Schema2 list, converting it if necessary.
	ToSchema2List() (*Schema2List, error)
}

// ListUpdate includes the fields which a manifest's UpdateInstances() method will modify.
type ListUpdate struct {
	Digest    digest.Digest
	Size      int64
	MediaType string
}

// dupStringSlice returns a deep copy of a slice of strings, or nil if the
// source slice is empty.
func dupStringSlice(list []string) []string {
	if len(list) == 0 {
		return nil
	}
	dup := make([]string, len(list))
	for i := range list {
		dup[i] = list[i]
	}
	return dup
}

// dupStringStringMape returns a deep copy of a map[string]string, always returning
// an initialized map, even if there are no contents to copy into it.
func dupStringStringMap(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	result := make(map[string]string)
	for k, v := range m {
		result[k] = v
	}
	return result
}

// ListFromBlob parses a list of manifests.
func ListFromBlob(manifest []byte, manifestMIMEType string) (List, error) {
	switch normalized := NormalizedMIMEType(manifestMIMEType); normalized {
	case DockerV2ListMediaType:
		return Schema2ListFromManifest(manifest)
	case imgspecv1.MediaTypeImageIndex:
		return OCI1IndexFromManifest(manifest)
	case DockerV2Schema1MediaType, DockerV2Schema1SignedMediaType:
		return nil, fmt.Errorf("Treating single images as manifest lists is not implemented")
	case imgspecv1.MediaTypeImageManifest:
		return nil, fmt.Errorf("Treating single images as manifest lists is not implemented")
	case DockerV2Schema2MediaType:
		return nil, fmt.Errorf("Treating single images as manifest lists is not implemented")
	default:
		return nil, fmt.Errorf("Unimplemented manifest MIME type %s (normalized as %s)", manifestMIMEType, normalized)
	}
}

// ChooseManifestInstanceFromList returns a digest of a manifest appropriate
// for the current system from the manifest available from src.
func ChooseManifestInstanceFromList(ctx *types.SystemContext, manifests List) (digest.Digest, error) {
	mt := manifests.MIMEType()
	if mt != DockerV2ListMediaType && mt != imgspecv1.MediaTypeImageIndex {
		return "", fmt.Errorf("Internal error: Trying to select an image from a non-manifest-list manifest type %s", mt)
	}
	return manifests.ChooseDigest(ctx)
}

// computeListID computes an image ID using the list of images referred to in a List.
func computeListID(manifests List) string {
	instances := manifests.Instances()
	hexes := make([]string, len(instances))
	for i, manifest := range manifests.Instances() {
		hexes[i] = manifest.Digest.Hex()
	}
	sort.Strings(hexes)
	return digest.FromBytes([]byte(strings.Join(hexes, ""))).Hex()
}

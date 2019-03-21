package manifest

import (
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/containers/image/types"
	"github.com/opencontainers/go-digest"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
)

// Schema2PlatformSpec describes the platform which a particular manifest is
// specialized for.
type Schema2PlatformSpec struct {
	Architecture string   `json:"architecture"`
	OS           string   `json:"os"`
	OSVersion    string   `json:"os.version,omitempty"`
	OSFeatures   []string `json:"os.features,omitempty"`
	Variant      string   `json:"variant,omitempty"`
	Features     []string `json:"features,omitempty"` // removed in OCI
}

// Schema2ManifestDescriptor references a platform-specific manifest.
type Schema2ManifestDescriptor struct {
	Schema2Descriptor
	Platform Schema2PlatformSpec `json:"platform"`
}

// Schema2List is a list of platform-specific manifests.
type Schema2List struct {
	SchemaVersion int                         `json:"schemaVersion"`
	MediaType     string                      `json:"mediaType"`
	Manifests     []Schema2ManifestDescriptor `json:"manifests"`
}

// MIMEType returns the MIME type of this particular manifest list.
func (list *Schema2List) MIMEType() string {
	return list.MediaType
}

// Instances returns a list of the manifests that this list knows of.
func (list *Schema2List) Instances() []types.BlobInfo {
	results := make([]types.BlobInfo, len(list.Manifests))
	for i, m := range list.Manifests {
		results[i] = types.BlobInfo{
			Digest:      m.Digest,
			Size:        m.Size,
			MediaType:   m.MediaType,
			Annotations: make(map[string]string),
			URLs:        dupStringSlice(m.URLs),
		}
	}
	return results
}

// UpdateInstances updates the sizes, digests, and media types of the manifests
// which the list catalogs.
func (list *Schema2List) UpdateInstances(updates []ListUpdate) error {
	if len(updates) != len(list.Manifests) {
		return errors.Errorf("incorrect number of update entries passed to Schema2List.UpdateInstances: expected %d, got %d", len(list.Manifests), len(updates))
	}
	for i := range updates {
		if err := updates[i].Digest.Validate(); err != nil {
			return errors.Wrapf(err, "update %d of %d passed to Schema2List.UpdateInstances contained an invalid digest", i+1, len(updates))
		}
		list.Manifests[i].Digest = updates[i].Digest
		if updates[i].Size < 0 {
			return errors.Errorf("update %d of %d passed to Schema2List.UpdateInstances had an invalid size (%d)", i+1, len(updates), updates[i].Size)
		}
		list.Manifests[i].Size = updates[i].Size
		if updates[i].MediaType == "" {
			return errors.Errorf("update %d of %d passed to Schema2List.UpdateInstances had no media type", i+1, len(updates))
		}
		list.Manifests[i].MediaType = updates[i].MediaType
	}
	return nil
}

// ChooseDigest parses blob as a schema2 manifest list, and returns the digest
// of the image which is appropriate for the current environment.
func (list *Schema2List) ChooseDigest(ctx *types.SystemContext) (digest.Digest, error) {
	wantedArch := runtime.GOARCH
	if ctx != nil && ctx.ArchitectureChoice != "" {
		wantedArch = ctx.ArchitectureChoice
	}
	wantedOS := runtime.GOOS
	if ctx != nil && ctx.OSChoice != "" {
		wantedOS = ctx.OSChoice
	}

	for _, d := range list.Manifests {
		if d.Platform.Architecture == wantedArch && d.Platform.OS == wantedOS {
			return d.Digest, nil
		}
	}
	return "", fmt.Errorf("no image found in manifest list for architecture %s, OS %s", wantedArch, wantedOS)
}

// ImageID computes a recommended image ID based on the list of images referred to by the manifest.
func (list *Schema2List) ImageID() string {
	return computeListID(list)
}

// Serialize returns the list in a blob format.
// NOTE: Serialize() does not in general reproduce the original blob if this object was loaded from one, even if no modifications were made!
func (list *Schema2List) Serialize() ([]byte, error) {
	buf, err := json.Marshal(list)
	if err != nil {
		return nil, errors.Wrapf(err, "error marshaling Schema2List %#v", list)
	}
	return buf, nil
}

// Schema2ListFromComponents creates a Schema2 manifest list instance from the
// supplied data.
func Schema2ListFromComponents(components []Schema2ManifestDescriptor) *Schema2List {
	list := Schema2List{
		SchemaVersion: 2,
		MediaType:     DockerV2ListMediaType,
		Manifests:     make([]Schema2ManifestDescriptor, len(components)),
	}
	for i, component := range components {
		m := Schema2ManifestDescriptor{
			Schema2Descriptor{
				MediaType: component.MediaType,
				Size:      component.Size,
				Digest:    component.Digest,
				URLs:      dupStringSlice(component.URLs),
			},
			Schema2PlatformSpec{
				Architecture: component.Platform.Architecture,
				OS:           component.Platform.OS,
				OSVersion:    component.Platform.OSVersion,
				OSFeatures:   dupStringSlice(component.Platform.OSFeatures),
				Variant:      component.Platform.Variant,
				Features:     dupStringSlice(component.Platform.Features),
			},
		}
		list.Manifests[i] = m
	}
	return &list
}

// Schema2ListClone creates a deep copy of the passed-in list.
func Schema2ListClone(list *Schema2List) *Schema2List {
	return Schema2ListFromComponents(list.Manifests)
}

// ToOCI1Index returns the list encoded as an OCI1 index.
func (list *Schema2List) ToOCI1Index() (*OCI1Index, error) {
	components := make([]imgspecv1.Descriptor, 0, len(list.Manifests))
	for _, manifest := range list.Manifests {
		converted := imgspecv1.Descriptor{
			MediaType: manifest.MediaType,
			Size:      manifest.Size,
			Digest:    manifest.Digest,
			URLs:      dupStringSlice(manifest.URLs),
			Platform: &imgspecv1.Platform{
				OS:           manifest.Platform.OS,
				Architecture: manifest.Platform.Architecture,
				OSFeatures:   dupStringSlice(manifest.Platform.OSFeatures),
				OSVersion:    manifest.Platform.OSVersion,
				Variant:      manifest.Platform.Variant,
			},
		}
		components = append(components, converted)
	}
	oci := OCI1IndexFromComponents(components, nil)
	return oci, nil
}

// ToSchema2List returns the list encoded as a Schema2 list.
func (list *Schema2List) ToSchema2List() (*Schema2List, error) {
	return Schema2ListClone(list), nil
}

// Schema2ListFromManifest creates a Schema2 manifest list instance from marshalled
// JSON, presumably generated by encoding a Schema2 manifest list.
func Schema2ListFromManifest(manifest []byte) (*Schema2List, error) {
	list := Schema2List{
		SchemaVersion: 2,
		MediaType:     DockerV2ListMediaType,
		Manifests:     []Schema2ManifestDescriptor{},
	}
	if err := json.Unmarshal(manifest, &list); err != nil {
		return nil, errors.Wrapf(err, "error unmarshaling Schema2List %q", string(manifest))
	}
	return &list, nil
}

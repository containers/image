package manifest

import (
	"encoding/json"

	"github.com/containers/image/types"
	"github.com/opencontainers/go-digest"
	"github.com/pkg/errors"
)

// Schema2Descriptor is a “descriptor” in docker/distribution schema 2.
type Schema2Descriptor struct {
	MediaType string        `json:"mediaType"`
	Size      int64         `json:"size"`
	Digest    digest.Digest `json:"digest"`
	URLs      []string      `json:"urls,omitempty"`
}

// Schema2 is a manifest in docker/distribution schema 2.
type Schema2 struct {
	SchemaVersion     int                 `json:"schemaVersion"`
	MediaType         string              `json:"mediaType"`
	ConfigDescriptor  Schema2Descriptor   `json:"config"`
	LayersDescriptors []Schema2Descriptor `json:"layers"`
}

// Schema2FromManifest creates a Schema2 manifest instance from a manifest blob.
func Schema2FromManifest(manifest []byte) (*Schema2, error) {
	s2 := Schema2{}
	if err := json.Unmarshal(manifest, &s2); err != nil {
		return nil, err
	}
	return &s2, nil
}

// Schema2FromComponents creates an Schema2 manifest instance from the supplied data.
func Schema2FromComponents(config Schema2Descriptor, layers []Schema2Descriptor) *Schema2 {
	return &Schema2{
		SchemaVersion:     2,
		MediaType:         DockerV2Schema2MediaType,
		ConfigDescriptor:  config,
		LayersDescriptors: layers,
	}
}

// Schema2Clone creates a copy of the supplied Schema2 manifest.
func Schema2Clone(src *Schema2) *Schema2 {
	copy := *src
	return &copy
}

// ConfigInfo returns a complete BlobInfo for the separate config object, or a BlobInfo{Digest:""} if there isn't a separate object.
func (m *Schema2) ConfigInfo() types.BlobInfo {
	return types.BlobInfo{Digest: m.ConfigDescriptor.Digest, Size: m.ConfigDescriptor.Size}
}

// LayerInfos returns a list of BlobInfos of layers referenced by this image, in order (the root layer first, and then successive layered layers).
// The Digest field is guaranteed to be provided; Size may be -1.
// WARNING: The list may contain duplicates, and they are semantically relevant.
func (m *Schema2) LayerInfos() []types.BlobInfo {
	blobs := []types.BlobInfo{}
	for _, layer := range m.LayersDescriptors {
		blobs = append(blobs, types.BlobInfo{
			Digest: layer.Digest,
			Size:   layer.Size,
			URLs:   layer.URLs,
		})
	}
	return blobs
}

// UpdateLayerInfos replaces the original layers with the specified BlobInfos (size+digest+urls), in order (the root layer first, and then successive layered layers)
func (m *Schema2) UpdateLayerInfos(layerInfos []types.BlobInfo) error {
	if len(m.LayersDescriptors) != len(layerInfos) {
		return errors.Errorf("Error preparing updated manifest: layer count changed from %d to %d", len(m.LayersDescriptors), len(layerInfos))
	}
	original := m.LayersDescriptors
	m.LayersDescriptors = make([]Schema2Descriptor, len(layerInfos))
	for i, info := range layerInfos {
		m.LayersDescriptors[i].MediaType = original[i].MediaType
		m.LayersDescriptors[i].Digest = info.Digest
		m.LayersDescriptors[i].Size = info.Size
		m.LayersDescriptors[i].URLs = info.URLs
	}
	return nil
}

// Serialize returns the manifest in a blob format.
// NOTE: Serialize() does not in general reproduce the original blob if this object was loaded from one, even if no modifications were made!
func (m *Schema2) Serialize() ([]byte, error) {
	return json.Marshal(*m)
}

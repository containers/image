package image

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/containers/image/types"
)

type descriptor struct {
	MediaType string `json:"mediaType"`
	Size      int64  `json:"size"`
	Digest    string `json:"digest"`
}

type manifestSchema2 struct {
	src               types.ImageSource // May be nil if configBlob is not nil
	configBlob        []byte            // If set, corresponds to contents of ConfigDescriptor.
	SchemaVersion     int               `json:"schemaVersion"`
	MediaType         string            `json:"mediaType"`
	ConfigDescriptor  descriptor        `json:"config"`
	LayersDescriptors []descriptor      `json:"layers"`
}

func manifestSchema2FromManifest(src types.ImageSource, manifest []byte) (genericManifest, error) {
	v2s2 := manifestSchema2{src: src}
	if err := json.Unmarshal(manifest, &v2s2); err != nil {
		return nil, err
	}
	return &v2s2, nil
}

func (m *manifestSchema2) serialize() ([]byte, error) {
	return json.Marshal(*m)
}

func (m *manifestSchema2) manifestMIMEType() string {
	return m.MediaType
}

// ConfigInfo returns a complete BlobInfo for the separate config object, or a BlobInfo{Digest:""} if there isn't a separate object.
// Note that the config object may not exist in the underlying storage in the return value of UpdatedImage! Use ConfigBlob() below.
func (m *manifestSchema2) ConfigInfo() types.BlobInfo {
	return types.BlobInfo{Digest: m.ConfigDescriptor.Digest, Size: m.ConfigDescriptor.Size}
}

// ConfigBlob returns the blob described by ConfigInfo, iff ConfigInfo().Digest != ""; nil otherwise.
// The result is cached; it is OK to call this however often you need.
func (m *manifestSchema2) ConfigBlob() ([]byte, error) {
	if m.configBlob == nil {
		if m.src == nil {
			return nil, fmt.Errorf("Internal error: neither src nor configBlob set in manifestSchema2")
		}
		stream, _, err := m.src.GetBlob(m.ConfigDescriptor.Digest)
		if err != nil {
			return nil, err
		}
		defer stream.Close()
		blob, err := ioutil.ReadAll(stream)
		if err != nil {
			return nil, err
		}
		hash := sha256.Sum256(blob)
		computedDigest := "sha256:" + hex.EncodeToString(hash[:])
		if computedDigest != m.ConfigDescriptor.Digest {
			return nil, fmt.Errorf("Download config.json digest %s does not match expected %s", computedDigest, m.ConfigDescriptor.Digest)
		}
		m.configBlob = blob
	}
	return m.configBlob, nil
}

// LayerInfos returns a list of BlobInfos of layers referenced by this image, in order (the root layer first, and then successive layered layers).
// The Digest field is guaranteed to be provided; Size may be -1.
// WARNING: The list may contain duplicates, and they are semantically relevant.
func (m *manifestSchema2) LayerInfos() []types.BlobInfo {
	blobs := []types.BlobInfo{}
	for _, layer := range m.LayersDescriptors {
		blobs = append(blobs, types.BlobInfo{Digest: layer.Digest, Size: layer.Size})
	}
	return blobs
}

func (m *manifestSchema2) imageInspectInfo() (*types.ImageInspectInfo, error) {
	config, err := m.ConfigBlob()
	if err != nil {
		return nil, err
	}
	v1 := &v1Image{}
	if err := json.Unmarshal(config, v1); err != nil {
		return nil, err
	}
	return &types.ImageInspectInfo{
		DockerVersion: v1.DockerVersion,
		Created:       v1.Created,
		Labels:        v1.Config.Labels,
		Architecture:  v1.Architecture,
		Os:            v1.OS,
	}, nil
}

// UpdatedImage returns a types.Image modified according to options.
// This does not change the state of the original Image object.
func (m *manifestSchema2) UpdatedImage(options types.ManifestUpdateOptions) (types.Image, error) {
	copy := *m // NOTE: This is not a deep copy, it still shares slices etc.
	if options.LayerInfos != nil {
		if len(copy.LayersDescriptors) != len(options.LayerInfos) {
			return nil, fmt.Errorf("Error preparing updated manifest: layer count changed from %d to %d", len(copy.LayersDescriptors), len(options.LayerInfos))
		}
		copy.LayersDescriptors = make([]descriptor, len(options.LayerInfos))
		for i, info := range options.LayerInfos {
			copy.LayersDescriptors[i].Digest = info.Digest
			copy.LayersDescriptors[i].Size = info.Size
		}
	}
	return memoryImageFromManifest(&copy), nil
}

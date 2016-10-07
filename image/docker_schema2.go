package image

import (
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
	src               types.ImageSource
	SchemaVersion     int          `json:"schemaVersion"`
	MediaType         string       `json:"mediaType"`
	ConfigDescriptor  descriptor   `json:"config"`
	LayersDescriptors []descriptor `json:"layers"`
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
func (m *manifestSchema2) ConfigInfo() types.BlobInfo {
	return types.BlobInfo{Digest: m.ConfigDescriptor.Digest, Size: m.ConfigDescriptor.Size}
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

func (m *manifestSchema2) config() ([]byte, error) {
	rawConfig, _, err := m.src.GetBlob(m.ConfigDescriptor.Digest)
	if err != nil {
		return nil, err
	}
	config, err := ioutil.ReadAll(rawConfig)
	rawConfig.Close()
	return config, err
}

func (m *manifestSchema2) imageInspectInfo() (*types.ImageInspectInfo, error) {
	config, err := m.config()
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
	copy := *m
	if options.LayerInfos != nil {
		if len(copy.LayersDescriptors) != len(options.LayerInfos) {
			return nil, fmt.Errorf("Error preparing updated manifest: layer count changed from %d to %d", len(copy.LayersDescriptors), len(options.LayerInfos))
		}
		for i, info := range options.LayerInfos {
			copy.LayersDescriptors[i].Digest = info.Digest
			copy.LayersDescriptors[i].Size = info.Size
		}
	}
	return memoryImageFromManifest(&copy), nil
}

package image

import (
	"encoding/json"
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

func (m *manifestSchema2) ConfigDigest() string {
	return m.ConfigDescriptor.Digest
}

func (m *manifestSchema2) LayerDigests() []string {
	blobs := []string{}
	for _, layer := range m.LayersDescriptors {
		blobs = append(blobs, layer.Digest)
	}
	return blobs
}

func (m *manifestSchema2) Config() ([]byte, error) {
	rawConfig, _, err := m.src.GetBlob(m.ConfigDescriptor.Digest)
	if err != nil {
		return nil, err
	}
	config, err := ioutil.ReadAll(rawConfig)
	rawConfig.Close()
	return config, err
}

func (m *manifestSchema2) ImageInspectInfo() (*types.ImageInspectInfo, error) {
	config, err := m.Config()
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
		Layers:        m.LayerDigests(),
	}, nil
}

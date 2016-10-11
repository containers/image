package image

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/containers/image/manifest"
	"github.com/containers/image/types"
)

var (
	validHex = regexp.MustCompile(`^([a-f0-9]{64})$`)
)

type fsLayersSchema1 struct {
	BlobSum string `json:"blobSum"`
}

type manifestSchema1 struct {
	Name         string            `json:"name"`
	Tag          string            `json:"tag"`
	Architecture string            `json:"architecture"`
	FSLayers     []fsLayersSchema1 `json:"fsLayers"`
	History      []struct {
		V1Compatibility string `json:"v1Compatibility"`
	} `json:"history"`
	SchemaVersion int `json:"schemaVersion"`
}

func manifestSchema1FromManifest(manifest []byte) (genericManifest, error) {
	mschema1 := &manifestSchema1{}
	if err := json.Unmarshal(manifest, mschema1); err != nil {
		return nil, err
	}
	if err := fixManifestLayers(mschema1); err != nil {
		return nil, err
	}
	// TODO(runcom): verify manifest schema 1, 2 etc
	//if len(m.FSLayers) != len(m.History) {
	//return nil, fmt.Errorf("length of history not equal to number of layers for %q", ref.String())
	//}
	//if len(m.FSLayers) == 0 {
	//return nil, fmt.Errorf("no FSLayers in manifest for %q", ref.String())
	//}
	return mschema1, nil
}

func (m *manifestSchema1) serialize() ([]byte, error) {
	// docker/distribution requires a signature even if the incoming data uses the nominally unsigned DockerV2Schema1MediaType.
	unsigned, err := json.Marshal(*m)
	if err != nil {
		return nil, err
	}
	return manifest.AddDummyV2S1Signature(unsigned)
}

func (m *manifestSchema1) manifestMIMEType() string {
	return manifest.DockerV2Schema1SignedMediaType
}

// ConfigInfo returns a complete BlobInfo for the separate config object, or a BlobInfo{Digest:""} if there isn't a separate object.
func (m *manifestSchema1) ConfigInfo() types.BlobInfo {
	return types.BlobInfo{}
}

// LayerInfos returns a list of BlobInfos of layers referenced by this image, in order (the root layer first, and then successive layered layers).
// The Digest field is guaranteed to be provided; Size may be -1.
// WARNING: The list may contain duplicates, and they are semantically relevant.
func (m *manifestSchema1) LayerInfos() []types.BlobInfo {
	layers := make([]types.BlobInfo, len(m.FSLayers))
	for i, layer := range m.FSLayers { // NOTE: This includes empty layers (where m.History.V1Compatibility->ThrowAway)
		layers[(len(m.FSLayers)-1)-i] = types.BlobInfo{Digest: layer.BlobSum, Size: -1}
	}
	return layers
}

func (m *manifestSchema1) config() ([]byte, error) {
	return []byte(m.History[0].V1Compatibility), nil
}

func (m *manifestSchema1) imageInspectInfo() (*types.ImageInspectInfo, error) {
	v1 := &v1Image{}
	config, err := m.config()
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(config, v1); err != nil {
		return nil, err
	}
	return &types.ImageInspectInfo{
		Tag:           m.Tag,
		DockerVersion: v1.DockerVersion,
		Created:       v1.Created,
		Labels:        v1.Config.Labels,
		Architecture:  v1.Architecture,
		Os:            v1.OS,
	}, nil
}

// UpdatedImage returns a types.Image modified according to options.
// This does not change the state of the original Image object.
func (m *manifestSchema1) UpdatedImage(options types.ManifestUpdateOptions) (types.Image, error) {
	copy := *m
	if options.LayerInfos != nil {
		// Our LayerInfos includes empty layers (where m.History.V1Compatibility->ThrowAway), so expect them to be included here as well.
		if len(copy.FSLayers) != len(options.LayerInfos) {
			return nil, fmt.Errorf("Error preparing updated manifest: layer count changed from %d to %d", len(copy.FSLayers), len(options.LayerInfos))
		}
		for i, info := range options.LayerInfos {
			// (docker push) sets up m.History.V1Compatibility->{Id,Parent} based on values of info.Digest,
			// but (docker pull) ignores them in favor of computing DiffIDs from uncompressed data, except verifying the child->parent links and uniqueness.
			// So, we don't bother recomputing the IDs in m.History.V1Compatibility.
			copy.FSLayers[(len(options.LayerInfos)-1)-i].BlobSum = info.Digest
		}
	}
	return memoryImageFromManifest(&copy), nil
}

// fixManifestLayers, after validating the supplied manifest
// (to use correctly-formatted IDs, and to not have non-consecutive ID collisions in manifest.History),
// modifies manifest to only have one entry for each layer ID in manifest.History (deleting the older duplicates,
// both from manifest.History and manifest.FSLayers).
// Note that even after this succeeds, manifest.FSLayers may contain duplicate entries
// (for Dockerfile operations which change the configuration but not the filesystem).
func fixManifestLayers(manifest *manifestSchema1) error {
	type imageV1 struct {
		ID     string
		Parent string
	}
	// Per the specification, we can assume that len(manifest.FSLayers) == len(manifest.History)
	imgs := make([]*imageV1, len(manifest.FSLayers))
	for i := range manifest.FSLayers {
		img := &imageV1{}

		if err := json.Unmarshal([]byte(manifest.History[i].V1Compatibility), img); err != nil {
			return err
		}

		imgs[i] = img
		if err := validateV1ID(img.ID); err != nil {
			return err
		}
	}
	if imgs[len(imgs)-1].Parent != "" {
		return errors.New("Invalid parent ID in the base layer of the image.")
	}
	// check general duplicates to error instead of a deadlock
	idmap := make(map[string]struct{})
	var lastID string
	for _, img := range imgs {
		// skip IDs that appear after each other, we handle those later
		if _, exists := idmap[img.ID]; img.ID != lastID && exists {
			return fmt.Errorf("ID %+v appears multiple times in manifest", img.ID)
		}
		lastID = img.ID
		idmap[lastID] = struct{}{}
	}
	// backwards loop so that we keep the remaining indexes after removing items
	for i := len(imgs) - 2; i >= 0; i-- {
		if imgs[i].ID == imgs[i+1].ID { // repeated ID. remove and continue
			manifest.FSLayers = append(manifest.FSLayers[:i], manifest.FSLayers[i+1:]...)
			manifest.History = append(manifest.History[:i], manifest.History[i+1:]...)
		} else if imgs[i].Parent != imgs[i+1].ID {
			return fmt.Errorf("Invalid parent ID. Expected %v, got %v.", imgs[i+1].ID, imgs[i].Parent)
		}
	}
	return nil
}

func validateV1ID(id string) error {
	if ok := validHex.MatchString(id); !ok {
		return fmt.Errorf("image ID %q is invalid", id)
	}
	return nil
}

package image

import (
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/containers/image/manifest"
	"github.com/containers/image/types"
	"github.com/opencontainers/go-digest"
	"github.com/pkg/errors"
)

type platformSpec struct {
	Architecture string   `json:"architecture"`
	OS           string   `json:"os"`
	OSVersion    string   `json:"os.version,omitempty"`
	OSFeatures   []string `json:"os.features,omitempty"`
	Variant      string   `json:"variant,omitempty"`
	Features     []string `json:"features,omitempty"` // removed in OCI
}

// A manifestDescriptor references a platform-specific manifest.
type manifestDescriptor struct {
	descriptor
	Platform platformSpec `json:"platform"`
}

type manifestList struct {
	SchemaVersion int                  `json:"schemaVersion"`
	MediaType     string               `json:"mediaType"`
	Manifests     []manifestDescriptor `json:"manifests"`
}

// chooseDigestFromManifestList parses blob as a schema2 manifest list,
// and returns the digest of the image appropriate for the current environment.
func chooseDigestFromManifestList(blob []byte) (digest.Digest, error) {
	list := manifestList{}
	if err := json.Unmarshal(blob, &list); err != nil {
		return "", err
	}
	for _, d := range list.Manifests {
		if d.Platform.Architecture == runtime.GOARCH && d.Platform.OS == runtime.GOOS {
			return d.Digest, nil
		}
	}
	return "", errors.New("no supported platform found in manifest list")
}

func manifestSchema2FromManifestList(src types.ImageSource, manblob []byte) (genericManifest, error) {
	targetManifestDigest, err := chooseDigestFromManifestList(manblob)
	if err != nil {
		return nil, err
	}
	manblob, mt, err := src.GetManifest(&targetManifestDigest)
	if err != nil {
		return nil, err
	}

	matches, err := manifest.MatchesDigest(manblob, targetManifestDigest)
	if err != nil {
		return nil, errors.Wrap(err, "Error computing manifest digest")
	}
	if !matches {
		return nil, errors.Errorf("Manifest image does not match selected manifest digest %s", targetManifestDigest)
	}

	return manifestInstanceFromBlob(src, manblob, mt)
}

// ChooseManifestInstanceFromManifestList returns a digest of a manifest appropriate
// for the current system from the manifest available from src.
func ChooseManifestInstanceFromManifestList(src types.UnparsedImage) (digest.Digest, error) {
	// For now this only handles manifest.DockerV2ListMediaType; we can generalize it later,
	// probably along with manifest list editing.
	blob, mt, err := src.Manifest()
	if err != nil {
		return "", err
	}
	if mt != manifest.DockerV2ListMediaType {
		return "", fmt.Errorf("Internal error: Trying to select an image from a non-manifest-list manifest type %s", mt)
	}
	return chooseDigestFromManifestList(blob)
}

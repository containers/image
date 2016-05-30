package docker

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/projectatomic/skopeo/types"
)

// Image is a Docker-specific implementation of types.Image with a few extra methods
// which are specific to Docker.
type Image struct {
	genericImage
}

// NewDockerImage returns a new Image interface type after setting up
// a client to the registry hosting the given image.
func NewDockerImage(img, certPath string, tlsVerify bool) (types.Image, error) {
	s, err := newDockerImageSource(img, certPath, tlsVerify)
	if err != nil {
		return nil, err
	}
	return &Image{genericImage{src: s}}, nil
}

// By construction a, docker.Image.genericImage.src must be a dockerImageSource.
// dockerSource returns it.
func (i *Image) dockerSource() (*dockerImageSource, error) {
	if src, ok := i.genericImage.src.(*dockerImageSource); ok {
		return src, nil
	}
	return nil, fmt.Errorf("Unexpected internal inconsistency, docker.Image not based on dockerImageSource")
}

// SourceRefFullName returns a fully expanded name for the repository this image is in.
func (i *Image) SourceRefFullName() (string, error) {
	src, err := i.dockerSource()
	if err != nil {
		return "", err
	}
	return src.ref.FullName(), nil
}

// GetRepositoryTags list all tags available in the repository. Note that this has no connection with the tag(s) used for this specific image, if any.
func (i *Image) GetRepositoryTags() ([]string, error) {
	src, err := i.dockerSource()
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf(tagsURL, src.ref.RemoteName())
	res, err := src.c.makeRequest("GET", url, nil, nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		// print url also
		return nil, fmt.Errorf("Invalid status code returned when fetching tags list %d", res.StatusCode)
	}
	type tagsRes struct {
		Tags []string
	}
	tags := &tagsRes{}
	if err := json.NewDecoder(res.Body).Decode(tags); err != nil {
		return nil, err
	}
	return tags.Tags, nil
}

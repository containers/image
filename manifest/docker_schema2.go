package manifest

import "github.com/opencontainers/go-digest"

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

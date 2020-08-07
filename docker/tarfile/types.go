package tarfile

import (
	internal "github.com/containers/image/v5/docker/internal/tarfile"
)

// Various data structures.

// Based on github.com/docker/docker/image/tarexport/tarexport.go
const (
	manifestFileName           = "manifest.json"
	legacyLayerFileName        = "layer.tar"
	legacyConfigFileName       = "json"
	legacyVersionFileName      = "VERSION"
	legacyRepositoriesFileName = "repositories"
)

// ManifestItem is an element of the array stored in the top-level manifest.json file.
type ManifestItem = internal.ManifestItem // All public members from the internal package remain accessible.

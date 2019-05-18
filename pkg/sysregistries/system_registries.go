package sysregistries

import (
	"path/filepath"

	"github.com/containers/image/pkg/sysregistriesv2"
	"github.com/containers/image/types"
)

// systemRegistriesConfPath is the path to the system-wide registry configuration file
// and is used to add/subtract potential registries for obtaining images.
// You can override this at build time with
// -ldflags '-X github.com/containers/image/sysregistries.systemRegistriesConfPath=$your_path'
var systemRegistriesConfPath = builtinRegistriesConfPath

// builtinRegistriesConfPath is the path to registry configuration file
// DO NOT change this, instead see systemRegistriesConfPath above.
const builtinRegistriesConfPath = "/etc/containers/registries.conf"

// GetRegistries returns an array of strings that contain the names
// of the registries as defined in the system-wide
// registries file.  it returns an empty array if none are
// defined
func GetRegistries(sys *types.SystemContext) ([]string, error) {
	return sysregistriesv2.UnqualifiedSearchRegistries(sys)
}

// GetInsecureRegistries returns an array of strings that contain the names
// of the insecure registries as defined in the system-wide
// registries file.  it returns an empty array if none are
// defined
func GetInsecureRegistries(sys *types.SystemContext) ([]string, error) {
	registries, err := sysregistriesv2.GetRegistries(sys)
	if err != nil {
		return nil, err
	}
	res := []string(nil)
	for _, reg := range registries {
		if reg.Insecure {
			// NOTE: Traditionally, could only contain host[:port] values; this will include arbitrary prefixes as well.
			// Strictly speaking, it would be more correct to skip namespace/repository/… values here.
			// Current known users either don't mind (CRI-O) or benefit from returning the full list (podman info).
			res = append(res, reg.Prefix)
		}
	}
	return res, nil
}

// RegistriesConfPath is the path to the system-wide registry configuration file
func RegistriesConfPath(ctx *types.SystemContext) string {
	path := systemRegistriesConfPath
	if ctx != nil {
		if ctx.SystemRegistriesConfPath != "" {
			path = ctx.SystemRegistriesConfPath
		} else if ctx.RootForImplicitAbsolutePaths != "" {
			path = filepath.Join(ctx.RootForImplicitAbsolutePaths, systemRegistriesConfPath)
		}
	}
	return path
}

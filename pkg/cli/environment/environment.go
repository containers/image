package environment

import (
	"errors"
	"os"

	"github.com/containers/image/v5/types"
)

// UpdateRegistriesConf sets the SystemRegistriesConfPath in the system
// context, unless already set.  Possible values are, in priority and only if
// set, the CONTAINERS_REGISTRIES_CONF or REGISTRIES_CONFIG_PATH environment
// variable.
func UpdateRegistriesConf(sys *types.SystemContext) error {
	if sys == nil {
		return errors.New("internal error: UpdateRegistriesConf: nil argument")
	}
	if sys.SystemRegistriesConfPath != "" {
		return nil
	}
	if envOverride, ok := os.LookupEnv("CONTAINERS_REGISTRIES_CONF"); ok {
		sys.SystemRegistriesConfPath = envOverride
		return nil
	}
	if envOverride, ok := os.LookupEnv("REGISTRIES_CONFIG_PATH"); ok {
		sys.SystemRegistriesConfPath = envOverride
		return nil
	}

	return nil
}

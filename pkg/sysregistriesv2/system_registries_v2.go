package sysregistriesv2

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/containers/image/types"
)

// systemRegistriesConfPath is the path to the system-wide registry
// configuration file and is used to add/subtract potential registries for
// obtaining images.  You can override this at build time with
// -ldflags '-X github.com/containers/image/sysregistries.systemRegistriesConfPath=$your_path'
var systemRegistriesConfPath = builtinRegistriesConfPath

// builtinRegistriesConfPath is the path to the registry configuration file.
// DO NOT change this, instead see systemRegistriesConfPath above.
const builtinRegistriesConfPath = "/etc/containers/registries.conf"

// Mirror represents a mirror. Mirrors can be used as pull-through caches for
// registries.
type Mirror struct {
	// The mirror's URL.
	URL string `toml:"url"`
	// If true, certs verification will be skipped and HTTP (non-TLS)
	// connections will be allowed.
	Insecure bool `toml:"insecure"`
}

// Registry represents a registry.
type Registry struct {
	// Serializable registry URL.
	URL string `toml:"url"`
	// The registry's mirrors.
	Mirrors []Mirror `toml:"mirror"`
	// If true, pulling from the registry will be blocked.
	Blocked bool `toml:"blocked"`
	// If true, certs verification will be skipped and HTTP (non-TLS)
	// connections will be allowed.
	Insecure bool `toml:"insecure"`
	// If true, the registry can be used when pulling an unqualified image.
	Search bool `toml:"unqualified-search"`
	// Prefix is used for matching images, and to translate one namespace to
	// another.  If `Prefix="example.com/bar"`, `URL="example.com/foo/bar"`
	// and we pull from "example.com/bar/myimage:latest", the image will
	// effectively be pulled from "example.com/foo/bar/myimage:latest".
	// If no Prefix is specified, it defaults to the specified URL.
	Prefix string `toml:"prefix"`
}

// backwards compatability to sysregistries v1
type v1TOMLregistries struct {
	Registries []string `toml:"registries"`
}

// tomlConfig is the data type used to unmarshal the toml config.
type tomlConfig struct {
	Registries []Registry `toml:"registry"`
	// backwards compatability to sysregistries v1
	V1Registries struct {
		Search   v1TOMLregistries `toml:"search"`
		Insecure v1TOMLregistries `toml:"insecure"`
		Block    v1TOMLregistries `toml:"block"`
	} `toml:"registries"`
}

// parseURL parses the input string, performs some sanity checks and returns
// the sanitized input string.  An error is returned in case parsing fails or
// or if URI scheme or user is set.
func parseURL(input string) (string, error) {
	trimmed := strings.TrimRight(input, "/")

	if trimmed == "" {
		return "", fmt.Errorf("invalid URL: cannot be empty")
	}

	uri, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid URL '%s': %v", input, err)
	}

	// Check if a URI SCheme is set.
	// Note that URLs that do not start with a slash after the scheme are
	// interpreted as `scheme:opaque[?query][#fragment]`.
	if uri.Scheme != "" && uri.Opaque == "" {
		return "", fmt.Errorf("invalid URL '%s': URI schemes are not supported", input)
	}

	uri, err = url.Parse("http://" + trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid URL '%s': sanitized URL did not parse: %v", input, err)
	}

	if uri.User != nil {
		return "", fmt.Errorf("invalid URL '%s': user/password are not supported", trimmed)
	}

	return trimmed, nil
}

// getV1Registries transforms v1 registries in the config into an array of v2
// registries of type Registry.
func getV1Registries(config *tomlConfig) ([]Registry, error) {
	regMap := make(map[string]*Registry)

	getRegistry := func(url string) (*Registry, error) { // Note: _pointer_ to a long-lived object
		var err error
		url, err = parseURL(url)
		if err != nil {
			return nil, err
		}
		reg, exists := regMap[url]
		if !exists {
			reg = &Registry{
				URL:     url,
				Mirrors: []Mirror{},
				Prefix:  url,
			}
			regMap[url] = reg
		}
		return reg, nil
	}

	for _, search := range config.V1Registries.Search.Registries {
		reg, err := getRegistry(search)
		if err != nil {
			return nil, err
		}
		reg.Search = true
	}
	for _, blocked := range config.V1Registries.Block.Registries {
		reg, err := getRegistry(blocked)
		if err != nil {
			return nil, err
		}
		reg.Blocked = true
	}
	for _, insecure := range config.V1Registries.Insecure.Registries {
		reg, err := getRegistry(insecure)
		if err != nil {
			return nil, err
		}
		reg.Insecure = true
	}

	registries := []Registry{}
	for _, reg := range regMap {
		registries = append(registries, *reg)
	}
	return registries, nil
}

func validateRegistries(regs []Registry) ([]Registry, error) {
	var registries []Registry
	for _, reg := range regs {
		var err error

		// make sure URL and Prefix are valid
		reg.URL, err = parseURL(reg.URL)
		if err != nil {
			return nil, err
		}
		if reg.Prefix == "" {
			reg.Prefix = reg.URL
		} else {
			reg.Prefix, err = parseURL(reg.Prefix)
			if err != nil {
				return nil, err
			}
		}

		// make sure mirrors are valid
		for _, mir := range reg.Mirrors {
			mir.URL, err = parseURL(mir.URL)
			if err != nil {
				return nil, err
			}
		}
		registries = append(registries, reg)
	}
	return registries, nil
}

// GetRegistries loads and returns the registries specified in the config.
func GetRegistries(ctx *types.SystemContext) ([]Registry, error) {
	config, err := loadRegistryConf(ctx)
	if err != nil {
		return nil, err
	}

	registries := config.Registries

	// backwards compatibility for v1 configs
	v1Registries, err := getV1Registries(config)
	if err != nil {
		return nil, err
	}
	if len(v1Registries) > 0 {
		if len(registries) > 0 {
			return nil, fmt.Errorf("mixing sysregistry v1/v2 is not supported")
		}
		registries = v1Registries
	}

	return validateRegistries(registries)
}

// FindUnqualifiedSearchRegistries returns all registries that are configured
// for unqualified image search (i.e., with Registry.Search == true).
func FindUnqualifiedSearchRegistries(registries []Registry) []Registry {
	unqualified := []Registry{}
	for _, reg := range registries {
		if reg.Search {
			unqualified = append(unqualified, reg)
		}
	}
	return unqualified
}

// FindRegistry returns the Registry with the longest prefix for ref.  If no
// Registry prefixes the image, nil is returned.
func FindRegistry(ref string, registries []Registry) *Registry {
	reg := Registry{}
	prefixLen := 0
	for _, r := range registries {
		if strings.HasPrefix(ref, r.Prefix) {
			length := len(r.Prefix)
			if length > prefixLen {
				reg = r
				prefixLen = length
			}
		}
	}
	if prefixLen != 0 {
		return &reg
	}
	return nil
}

// Reads the global registry file from the filesystem. Returns a byte array.
func readRegistryConf(ctx *types.SystemContext) ([]byte, error) {
	dirPath := systemRegistriesConfPath
	if ctx != nil {
		if ctx.SystemRegistriesConfPath != "" {
			dirPath = ctx.SystemRegistriesConfPath
		} else if ctx.RootForImplicitAbsolutePaths != "" {
			dirPath = filepath.Join(ctx.RootForImplicitAbsolutePaths, systemRegistriesConfPath)
		}
	}
	configBytes, err := ioutil.ReadFile(dirPath)
	return configBytes, err
}

// Used in unittests to parse custom configs without a types.SystemContext.
var readConf = readRegistryConf

// Loads the registry configuration file from the filesystem and then unmarshals
// it.  Returns the unmarshalled object.
func loadRegistryConf(ctx *types.SystemContext) (*tomlConfig, error) {
	config := &tomlConfig{}

	configBytes, err := readConf(ctx)
	if err != nil {
		return nil, err
	}

	err = toml.Unmarshal(configBytes, &config)
	return config, err
}

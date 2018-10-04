package sysregistriesv2

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/BurntSushi/toml"
	"github.com/containers/image/types"
	"github.com/sirupsen/logrus"
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

// InvalidRegistries represents an invalid registry configurations.  An example
// is when "registry.com" is defined multiple times in the configuration but
// with conflicting security settings.
type InvalidRegistries struct {
	s string
}

// Error returns the error string.
func (e *InvalidRegistries) Error() string {
	return e.s
}

// parseURL parses the input string, performs some sanity checks and returns
// the sanitized input string.  An error is returned in case parsing fails or
// or if URI scheme or user is set.
func parseURL(input string) (string, error) {
	trimmed := strings.TrimRight(input, "/")

	if trimmed == "" {
		return "", &InvalidRegistries{s: "invalid URL: cannot be empty"}
	}

	// Ultimately, we expect input of the form example.com[/namespace/…], a prefix
	// of a fully-expended reference (containers/image/docker/Reference.String()).
	// c/image/docker/Reference does not currently provide such a parser.
	// So, we use url.Parse("http://"+trimmed) below to ~verify the format, possibly
	// letting some invalid input in, trading that off for a simpler parser.
	//
	// url.Parse("http://"+trimmed) is, sadly, too permissive, notably for
	// trimmed == "http://example.com/…", url.Parse("http://http://example.com/…")
	// is accepted and parsed as
	// {Scheme: "http", Host: "http:", Path: "//example.com/…"}.
	//
	// So, first we do an explicit check for an unwanted scheme prefix:

	// This will parse trimmed=="http://example.com/…" with Scheme: "http".  Perhaps surprisingly,
	// it also succeeds for the input we want to accept, in different ways:
	// "example.com" -> {Scheme:"", Opaque:"", Path:"example.com"}
	// "example.com/repo" -> {Scheme:"", Opaque:"", Path:"example.com/repo"}
	// "example.com:5000" -> {Scheme:"example.com", Opaque:"5000"}
	// "example.com:5000/repo" -> {Scheme:"example.com", Opaque:"5000/repo"}
	uri, err := url.Parse(trimmed)
	if err != nil {
		return "", &InvalidRegistries{s: fmt.Sprintf("invalid URL '%s': %v", input, err)}
	}

	// Check if a URI Scheme is set.
	// Note that URLs that do not start with a slash after the scheme are
	// interpreted as `scheme:opaque[?query][#fragment]`; see above for examples.
	if uri.Scheme != "" && uri.Opaque == "" {
		msg := fmt.Sprintf("invalid URL '%s': URI schemes are not supported", input)
		return "", &InvalidRegistries{s: msg}
	}

	uri, err = url.Parse("http://" + trimmed)
	if err != nil {
		msg := fmt.Sprintf("invalid URL '%s': sanitized URL did not parse: %v", input, err)
		return "", &InvalidRegistries{s: msg}
	}

	if uri.User != nil {
		msg := fmt.Sprintf("invalid URL '%s': user/password are not supported", trimmed)
		return "", &InvalidRegistries{s: msg}
	}

	return trimmed, nil
}

// getV1Registries transforms v1 registries in the config into an array of v2
// registries of type Registry.
func getV1Registries(config *tomlConfig) ([]Registry, error) {
	regMap := make(map[string]*Registry)

	getRegistry := func(prefix string, isURL bool) (*Registry, error) { // Note: _pointer_ to a long-lived object
		url, err := parseURL(prefix)
		if err != nil {
			if isURL {
				return nil, err
			}
			url = prefix
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
		reg, err := getRegistry(search, true)
		if err != nil {
			return nil, err
		}
		reg.Search = true
	}
	for _, blocked := range config.V1Registries.Block.Registries {
		reg, err := getRegistry(blocked, false)
		if err != nil {
			return nil, err
		}
		reg.Blocked = true
	}
	for _, insecure := range config.V1Registries.Insecure.Registries {
		reg, err := getRegistry(insecure, false)
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

// registryIsMatch returns true if flags in the registry entry indicate that
// its URL might not be used as-is, but will have URLs matched against it in
// order to selectively apply configuration flags to an unlisted URL.
func registryIsMatch(reg Registry) bool {
	return reg.Insecure || reg.Blocked
}

// registryMatches returns true if the candidate location matches the pattern.
// Recognized patterns include:
// * IPv4[/class][:port][/path]
// * IPv6[/class][:port][/path]
// * hostname[:port][/path]
// where IPv4, IPv6, and hostname may include wildcard characters.  The path
// is implicitly terminated with "/", and individual components may also
// include wildcard characters.
func registryMatches(pattern, candidate string) (bool, error) {
	defaultRegistryPort := "5000"
	phost, pport, ppath, err := parseMatchPattern(pattern)
	if err != nil {
		return false, err
	}
	chost, cport, cpath, err := parseMatchCandidate(candidate)
	if err != nil {
		return false, err
	}
	// Check the host component
	_, pipnet, perr := net.ParseCIDR(phost)
	cip := net.ParseIP(chost)
	if cip == nil && perr == nil {
		// can't match a non-IP against a CIDR address
		return false, nil
	}
	if perr == nil {
		// pattern is CIDR and the candidate is an IP
		if !pipnet.Contains(cip) {
			return false, nil
		}
	} else {
		// pattern and candidate are both names
		hosts, err := path.Match(phost, chost)
		if err != nil {
			return false, fmt.Errorf("error matching candidate host %q against pattern host %q: %v", chost, phost, err)
		}
		if !hosts {
			return false, nil
		}
	}
	// Check the ports, if specified, or default ports
	if pport == "" {
		pport = defaultRegistryPort
	}
	if cport == "" {
		cport = defaultRegistryPort
	}
	if pport != "*" && pport != cport {
		return false, nil
	}
	// Compare paths, only so far as the pattern supplies one
	pdir, ppath := path.Split(ppath)
	cdir, cpath := path.Split(cpath)
	for pdir != "" {
		if cdir == "" {
			return false, nil
		}
		matches, err := path.Match(pdir, cdir)
		if err != nil {
			return false, fmt.Errorf("error comparing path component %q to pattern component %q: %v", cdir, pdir, err)
		}
		if !matches {
			return false, nil
		}
		pdir, ppath = path.Split(ppath)
		cdir, cpath = path.Split(cpath)
	}
	return true, nil
}

// isBogusIP checks if the passed-in value looks superficially like an IPv4
// address, but contains an invalid octet value.
func isBogusIP(candidate string) bool {
	components := strings.Split(candidate, ".")
	// no problem if there aren't 4 .-separated components
	if len(components) != 4 {
		return false
	}
	for _, i := range components {
		// or if any of them don't look like numbers
		if len(strings.FieldsFunc(i, func(r rune) bool { return unicode.IsDigit(r) })) != 0 {
			return false
		}
	}
	for _, i := range components {
		// but if they do, and they're invalid values, that's a problem
		if _, err := strconv.ParseUint(i, 10, 8); err != nil {
			return true
		}
	}
	return false
}

// parseMatchPattern splits up matching information for registries.
func parseMatchPattern(pattern string) (host, port, path string, err error) {
	rest := pattern
	if rest == "" {
		return "", "", "", fmt.Errorf(`empty registry pattern`)
	}

	// Isolate the host part.
	if rest[0] == '[' {
		// This may be a plain address, or a CIDR address.
		i := strings.Index(rest, "]")
		if i == -1 {
			return "", "", "", fmt.Errorf(`no expected "]" found after "[" in pattern %q`, pattern)
		}
		host = rest[1:i]
		rest = rest[i+1:]
		if _, _, err := net.ParseCIDR(host); err != nil {
			if net.ParseIP(host) == nil {
				return "", "", "", fmt.Errorf(`unable to parse IP or CIDR specification %q in pattern %q`, host, pattern)
			}
		}
	} else {
		// See if this is a CIDR address and prefix length.
		i := strings.IndexAny(rest, "/:")
		if i == -1 {
			// No separators at all; we're done.
			return rest, "", "", nil
		}
		if rest[i] == '/' {
			// If the next segment and this segment parse as a CIDR pair, keep them together.
			maybeCIDR := ""
			j := strings.IndexAny(rest[i+1:], "/:")
			if j != -1 {
				maybeCIDR = rest[:i+1+j]
			} else {
				maybeCIDR = rest
			}
			if _, _, err := net.ParseCIDR(maybeCIDR); err == nil {
				if maybeCIDR == rest {
					// The whole thing is a CIDR pair; we're done here.
					return rest, "", "", nil
				}
				// Continue parsing after the CIDR pair.
				host = maybeCIDR
				rest = rest[len(maybeCIDR):]
			} else {
				// It's just a host and whatever comes after.
				if isBogusIP(rest[:i]) {
					return "", "", "", fmt.Errorf(`unable to parse IP or CIDR specification %q in pattern %q`, rest[:i], pattern)
				}
				host = rest[:i]
				rest = rest[i:]
			}
		} else {
			// It's just a host and whatever comes after.
			if isBogusIP(rest[:i]) {
				return "", "", "", fmt.Errorf(`unable to parse IP or CIDR specification %q in pattern %q`, rest[:i], pattern)
			}
			host = rest[:i]
			rest = rest[i:]
		}
	}

	// The next part is optionally a port.
	if rest != "" && rest[0] == ':' {
		i := strings.Index(rest, "/")
		if i == -1 {
			port = rest[1:]
		} else {
			port = rest[1:i]
		}
		_, err := strconv.ParseUint(port, 10, 16)
		if err != nil {
			return "", "", "", fmt.Errorf("error parsing port %q in %q: %v", port, pattern, err)
		}
		rest = rest[len(port)+1:]
	}

	// The rest is a path.
	return host, port, rest, nil
}

// parseMatchCandidate splits the candidate registry information.
func parseMatchCandidate(candidate string) (host, port, path string, err error) {
	rest := candidate
	if rest == "" {
		return "", "", "", fmt.Errorf(`empty candidate name`)
	}

	if rest[0] == '[' {
		// This may be an address.
		i := strings.Index(rest, "]")
		if i == -1 {
			return "", "", "", fmt.Errorf(`no expected "]" found after "[" in candidate %q`, candidate)
		}
		if net.ParseIP(rest[1:i]) == nil {
			return "", "", "", fmt.Errorf(`%q is not a valid IP address while parsing candidate %q`, rest[1:i], candidate)
		}
		host = rest[1:i]
		rest = rest[i+1:]
	} else {
		i := strings.IndexAny(rest, "/:")
		if i == -1 {
			// No separators at all; we're done.
			return rest, "", "", nil
		}
		host = rest[:i]
		rest = rest[i:]
	}

	// The next part is optionally a port.
	if rest != "" && rest[0] == ':' {
		i := strings.Index(rest, "/")
		if i == -1 {
			port = rest[1:]
		} else {
			port = rest[1:i]
		}
		_, err := strconv.ParseUint(port, 10, 16)
		if err != nil {
			return "", "", "", fmt.Errorf("error parsing port %q in %q: %v", port, candidate, err)
		}
		rest = rest[len(port)+1:]
	}

	// The rest is a path.
	return host, port, rest, nil
}

// postProcessRegistries checks the consistency of all registries (e.g., set
// the Prefix to URL if not set) and applies conflict checks.  It returns an
// array of cleaned registries and error in case of conflicts.
func postProcessRegistries(regs []Registry) ([]Registry, error) {
	var registries []Registry
	regMap := make(map[string][]Registry)

	for _, reg := range regs {
		var err error

		// make sure URL and Prefix are valid
		if !registryIsMatch(reg) {
			reg.URL, err = parseURL(reg.URL)
			if err != nil {
				return nil, err
			}
		}

		if reg.Prefix == "" {
			reg.Prefix = reg.URL
		} else {
			if !registryIsMatch(reg) {
				reg.Prefix, err = parseURL(reg.Prefix)
				if err != nil {
					return nil, err
				}
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
		regMap[reg.URL] = append(regMap[reg.URL], reg)
	}

	// Given a registry can be mentioned multiple times (e.g., to have
	// multiple prefixes backed by different mirrors), we need to make sure
	// there are no conflicts among them.
	//
	// Note: we need to iterate over the registries array to ensure a
	// deterministic behavior which is not guaranteed by maps.
	for _, reg := range registries {
		others, _ := regMap[reg.URL]
		for _, other := range others {
			if reg.Insecure != other.Insecure {
				msg := fmt.Sprintf("registry '%s' is defined multiple times with conflicting 'insecure' setting", reg.URL)

				return nil, &InvalidRegistries{s: msg}
			}
			if reg.Blocked != other.Blocked {
				msg := fmt.Sprintf("registry '%s' is defined multiple times with conflicting 'blocked' setting", reg.URL)
				return nil, &InvalidRegistries{s: msg}
			}
		}
	}

	return registries, nil
}

// getConfigPath returns the system-registries config path if specified.
// Otherwise, systemRegistriesConfPath is returned.
func getConfigPath(ctx *types.SystemContext) string {
	confPath := systemRegistriesConfPath
	if ctx != nil {
		if ctx.SystemRegistriesConfPath != "" {
			confPath = ctx.SystemRegistriesConfPath
		} else if ctx.RootForImplicitAbsolutePaths != "" {
			confPath = filepath.Join(ctx.RootForImplicitAbsolutePaths, systemRegistriesConfPath)
		}
	}
	return confPath
}

// configMutex is used to synchronize concurrent accesses to configCache.
var configMutex = sync.Mutex{}

// configCache caches already loaded configs with config paths as keys and is
// used to avoid redudantly parsing configs. Concurrent accesses to the cache
// are synchronized via configMutex.
var configCache = make(map[string][]Registry)

// GetRegistries loads and returns the registries specified in the config.
func GetRegistries(ctx *types.SystemContext) ([]Registry, error) {
	configPath := getConfigPath(ctx)

	configMutex.Lock()
	defer configMutex.Unlock()
	// if the config has already been loaded, return the cached registries
	if registries, inCache := configCache[configPath]; inCache {
		return registries, nil
	}

	// load the config
	config, err := loadRegistryConf(configPath)
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
			return nil, &InvalidRegistries{s: "mixing sysregistry v1/v2 is not supported"}
		}
		registries = v1Registries
	}

	registries, err = postProcessRegistries(registries)
	if err != nil {
		return nil, err
	}

	// populate the cache
	configCache[configPath] = registries

	return registries, err
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
		if registryIsMatch(r) {
			matches, err := registryMatches(r.Prefix, ref)
			if err != nil {
				logrus.Debugf("error checking if %q matches %q: %v", ref, r.Prefix, err)
			} else {
				if matches {
					length := len(r.Prefix)
					if length > prefixLen {
						reg = r
						reg.URL = ref
						prefixLen = length
					}
				}
			}
		}
	}
	if prefixLen != 0 {
		return &reg
	}
	return nil
}

// Reads the global registry file from the filesystem. Returns a byte array.
func readRegistryConf(configPath string) ([]byte, error) {
	configBytes, err := ioutil.ReadFile(configPath)
	return configBytes, err
}

// Used in unittests to parse custom configs without a types.SystemContext.
var readConf = readRegistryConf

// Loads the registry configuration file from the filesystem and then unmarshals
// it.  Returns the unmarshalled object.
func loadRegistryConf(configPath string) (*tomlConfig, error) {
	config := &tomlConfig{}

	configBytes, err := readConf(configPath)
	if err != nil {
		return nil, err
	}

	err = toml.Unmarshal(configBytes, &config)
	return config, err
}

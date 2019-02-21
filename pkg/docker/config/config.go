package config

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/containers/image/types"
	helperclient "github.com/docker/docker-credential-helpers/client"
	"github.com/docker/docker-credential-helpers/credentials"
	"github.com/docker/docker/pkg/homedir"
	"github.com/pkg/errors"
)

// Auth holds per-URI-authority credentials.
type Auth struct {
	Username string
	Secret   string
}

// MarshalJSON interface for Auth.
func (auth Auth) MarshalJSON() ([]byte, error) {
	return json.Marshal(base64.StdEncoding.EncodeToString([]byte(auth.Username + ":" + auth.Secret)))
}

// UnmarshalJSON interface for Auth.
func (auth *Auth) UnmarshalJSON(b []byte) error {
	var base64auth string
	err := json.Unmarshal(b, &base64auth)
	if err != nil {
		return err
	}

	decoded, err := base64.StdEncoding.DecodeString(base64auth)
	if err != nil {
		return err
	}

	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		// if it's invalid just skip, as docker does
		auth.Username = ""
		auth.Secret = ""
		return nil
	}

	auth.Username = parts[0]
	auth.Secret = strings.Trim(parts[1], "\x00")
	return nil
}

// AuthEntry holds a value from the configurations Auths map.
type AuthEntry struct {
	// Auth holds the the credentials for this auth entry.
	Auth Auth `json:"auth"`

	// Email holds an email address for this auth entry.
	Email string `json:"email,omitempty"`
}

// Config holds the full authorization configuration.
type Config struct {
	// Auths holds a map of per-URI-authority Auth values.
	Auths map[string]AuthEntry `json:"auths"`

	// CredHelpers holds a map of per-URI-authority credential helpers.
	CredHelpers map[string]string `json:"credHelpers,omitempty"`
}

var (
	defaultPerUIDPathFormat = filepath.FromSlash("/run/containers/%d/auth.json")
	xdgRuntimeDirPath       = filepath.FromSlash("containers/auth.json")
	dockerHomePath          = filepath.FromSlash(".docker/config.json")
	dockerLegacyHomePath    = ".dockercfg"

	// ErrNotLoggedIn is returned for users not logged into a registry
	// that they are trying to logout of
	ErrNotLoggedIn = errors.New("not logged in")
)

// AuthStore implements a credentials store based on Docker's
// config.json file format.  It stores the data in memory, so multiple
// reads will only hit the disk once.  That means it is not safe to
// simultaneously edit different AuthStore instances which are backed
// by the same file.  The store implements internal locking so
// concurrent access to a single AuthStore instance is safe (although
// direct access to the Config property bypasses the lock).
type AuthStore struct {
	// Path holks the path to the config file.  If left empty, it will
	// be poplutated with a reasonable default or auto-detected existing
	// file during the first save or load call.
	Path string

	// Config holds the structured authorization configuration.
	Config *Config

	original []byte
	sync.Mutex
}

// Add appends credentials to the store.
func (s *AuthStore) Add(creds *credentials.Credentials) error {
	s.Lock()
	defer s.Unlock()

	if s.Config == nil {
		err := s.load()
		if err != nil && !credentials.IsErrCredentialsNotFound(err) {
			return err
		}
	}

	if credHelper, exists := s.Config.CredHelpers[creds.ServerURL]; exists {
		helperName := fmt.Sprintf("docker-credential-%s", credHelper)
		p := helperclient.NewShellProgramFunc(helperName)
		return helperclient.Store(p, creds)
	}

	newAuth := AuthEntry{
		Auth: Auth{
			Username: creds.Username,
			Secret:   creds.Secret,
		},
	}
	if newAuth != s.Config.Auths[creds.ServerURL] {
		s.Config.Auths[creds.ServerURL] = newAuth
		return s.save()
	}

	return nil
}

// Get retrieves credentials from the store.  It returns the username
// and secret as strings.
func (s *AuthStore) Get(serverURL string) (string, string, error) {
	s.Lock()
	defer s.Unlock()

	if s.Config == nil {
		err := s.load()
		if err != nil {
			return "", "", err
		}
	}

	// First try cred helpers. They should always be normalized.
	if credHelper, exists := s.Config.CredHelpers[serverURL]; exists {
		helperName := fmt.Sprintf("docker-credential-%s", credHelper)
		p := helperclient.NewShellProgramFunc(helperName)
		creds, err := helperclient.Get(p, serverURL)
		if err != nil {
			return "", "", err
		}
		return creds.Username, creds.Secret, nil
	}

	// I'm feeling lucky
	if val, exists := s.Config.Auths[serverURL]; exists {
		return val.Auth.Username, val.Auth.Secret, nil
	}

	// bad luck; let's normalize the entries first
	serverURL = normalizeRegistry(serverURL)
	normalizedAuths := map[string]Auth{}
	for k, v := range s.Config.Auths {
		normalizedAuths[normalizeRegistry(k)] = v.Auth
	}
	if val, exists := normalizedAuths[serverURL]; exists {
		return val.Username, val.Secret, nil
	}
	return "", "", credentials.NewErrCredentialsNotFound()
}

// Delete removes credentials from the store.
func (s *AuthStore) Delete(serverURL string) error {
	s.Lock()
	defer s.Unlock()

	if s.Config == nil {
		err := s.load()
		if err != nil {
			if credentials.IsErrCredentialsNotFound(err) {
				return nil
			}
			return err
		}
	}

	// First try cred helpers.
	if credHelper, exists := s.Config.CredHelpers[serverURL]; exists {
		helperName := fmt.Sprintf("docker-credential-%s", credHelper)
		p := helperclient.NewShellProgramFunc(helperName)
		return helperclient.Erase(p, serverURL)
	}

	changed := false
	if _, ok := s.Config.Auths[serverURL]; ok {
		delete(s.Config.Auths, serverURL)
		changed = true
	} else if _, ok := s.Config.Auths[normalizeRegistry(serverURL)]; ok {
		delete(s.Config.Auths, normalizeRegistry(serverURL))
		changed = true
	}
	if changed {
		return s.save()
	}

	return nil
}

// SetAuthentication stores the username and password in the auth.json file
//
// Deprecated: Use an AuthStore.
func SetAuthentication(sys *types.SystemContext, registry, username, password string) error {
	path, err := getPathWithContext(sys)
	if err != nil {
		return err
	}

	authStore := &AuthStore{Path: path}
	return authStore.Add(&credentials.Credentials{
		ServerURL: registry,
		Username:  username,
		Secret:    password,
	})
}

// GetAuthentication returns the registry credentials stored in
// either auth.json file or .docker/config.json
// If an entry is not found empty strings are returned for the username and password
//
// Deprecated: Use an AuthStore.
func GetAuthentication(sys *types.SystemContext, registry string) (string, string, error) {
	if sys != nil && sys.DockerAuthConfig != nil {
		return sys.DockerAuthConfig.Username, sys.DockerAuthConfig.Password, nil
	}

	path, err := getPathWithContext(sys)
	if err != nil {
		return "", "", err
	}

	authStore := &AuthStore{Path: path}
	username, secret, err := authStore.Get(registry)
	if err == nil {
		return username, secret, nil
	}
	if !credentials.IsErrCredentialsNotFound(err) {
		return "", "", err
	}

	home := homedir.Get()
	for _, relPath := range []string{dockerHomePath, dockerLegacyHomePath} {
		path := filepath.Join(home, relPath)
		raw, err := ioutil.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", "", err
		}

		if relPath == dockerLegacyHomePath {
			authStore.Config = &Config{
				CredHelpers: map[string]string{},
			}

			if err = json.Unmarshal(raw, &authStore.Config.Auths); err != nil {
				return "", "", errors.Wrapf(err, "error unmarshaling JSON at %q", path)
			}
		} else {
			if err = json.Unmarshal(raw, &authStore.Config); err != nil {
				return "", "", errors.Wrapf(err, "error unmarshaling JSON at %q\n%s", path, string(raw))
			}
		}

		username, secret, err = authStore.Get(registry)
		if err == nil {
			return username, secret, nil
		}
	}

	return "", "", nil
}

// GetUserLoggedIn returns the username logged in to registry from either
// auth.json or XDG_RUNTIME_DIR
// Used to tell the user if someone is logged in to the registry when logging in
//
// Deprecated: Use an AuthStore.
func GetUserLoggedIn(sys *types.SystemContext, registry string) (string, error) {
	username, _, err := GetAuthentication(sys, registry)
	if err != nil {
		return "", err
	}
	return username, nil
}

// RemoveAuthentication deletes the credentials stored in auth.json
//
// Deprecated: Use an AuthStore.
func RemoveAuthentication(sys *types.SystemContext, registry string) error {
	path, err := getPathWithContext(sys)
	if err != nil {
		return err
	}

	authStore := &AuthStore{Path: path}
	return authStore.Delete(registry)
}

// RemoveAllAuthentication deletes all the credentials stored in auth.json
//
// Deprecated: Use an AuthStore.
func RemoveAllAuthentication(sys *types.SystemContext) error {
	path, err := getPathWithContext(sys)
	if err != nil {
		return err
	}

	authStore := &AuthStore{
		Path: path,
		Config: &Config{
			Auths:       map[string]AuthEntry{},
			CredHelpers: map[string]string{},
		},
	}
	return authStore.save()
}

// getPath gets the path of the credentials JSON file.
// If the flag is not set and XDG_RUNTIME_DIR is set, the auth.json file is saved in XDG_RUNTIME_DIR/containers
// Otherwise, the auth.json file is stored in /run/containers/UID
func getPath() (string, error) {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir != "" {
		// This function does not in general need to separately check that the returned path exists; that’s racy, and callers will fail accessing the file anyway.
		// We are checking for os.IsNotExist here only to give the user better guidance what to do in this special case.
		_, err := os.Stat(runtimeDir)
		if os.IsNotExist(err) {
			// This means the user set the XDG_RUNTIME_DIR variable and either forgot to create the directory
			// or made a typo while setting the environment variable,
			// so return an error referring to $XDG_RUNTIME_DIR instead of xdgRuntimeDirPath inside.
			return "", errors.Wrapf(err, "%q directory set by $XDG_RUNTIME_DIR does not exist. Either create the directory or unset $XDG_RUNTIME_DIR.", runtimeDir)
		} // else ignore err and let the caller fail accessing xdgRuntimeDirPath.
		return filepath.Join(runtimeDir, xdgRuntimeDirPath), nil
	}
	return fmt.Sprintf(defaultPerUIDPathFormat, os.Getuid()), nil
}

func getPathWithContext(sys *types.SystemContext) (string, error) {
	if sys != nil {
		if sys.AuthFilePath != "" {
			return sys.AuthFilePath, nil
		} else if sys.RootForImplicitAbsolutePaths != "" {
			return filepath.Join(sys.RootForImplicitAbsolutePaths, fmt.Sprintf(defaultPerUIDPathFormat, os.Getuid())), nil
		}
	}

	return getPath()
}

func (s *AuthStore) load() error {
	var err error
	if s.Path == "" {
		s.Path, err = getPath()
		if err != nil {
			return err
		}
	}

	s.original, err = ioutil.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			s.Config = &Config{
				Auths:       map[string]AuthEntry{},
				CredHelpers: map[string]string{},
			}
			return credentials.NewErrCredentialsNotFound()
		}
		return err
	}

	if err = json.Unmarshal(s.original, &s.Config); err != nil {
		return errors.Wrapf(err, "error unmarshaling JSON at %q", s.Path)
	}

	return nil
}

func (s *AuthStore) save() error {
	var err error
	if s.Path == "" {
		s.Path, err = getPath()
		if err != nil {
			return err
		}
	}

	dir := filepath.Dir(s.Path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err = os.MkdirAll(dir, 0700); err != nil {
			return errors.Wrapf(err, "error creating directory %q", dir)
		}
	}

	// FIXME: this comparison is racy without a flock guard or similar
	data, err := ioutil.ReadFile(s.Path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	if !bytes.Equal(data, s.original) {
		return errors.Errorf("%q modified since we loaded it", s.Path)
	}

	newConfig, err := json.MarshalIndent(s.Config, "", "\t")
	if err != nil {
		return errors.Wrapf(err, "error marshaling JSON %q", s.Path)
	}

	tempPath := s.Path + ".tmp"
	if err = ioutil.WriteFile(tempPath, newConfig, 0600); err != nil {
		return err
	}

	return os.Rename(tempPath, s.Path)
}

// convertToHostname converts a registry url which has http|https prepended
// to just an hostname.
// Copied from github.com/docker/docker/registry/auth.go
func convertToHostname(url string) string {
	stripped := url
	if strings.HasPrefix(url, "http://") {
		stripped = strings.TrimPrefix(url, "http://")
	} else if strings.HasPrefix(url, "https://") {
		stripped = strings.TrimPrefix(url, "https://")
	}

	nameParts := strings.SplitN(stripped, "/", 2)

	return nameParts[0]
}

func normalizeRegistry(registry string) string {
	normalized := convertToHostname(registry)
	switch normalized {
	case "registry-1.docker.io", "docker.io":
		return "index.docker.io"
	}
	return normalized
}

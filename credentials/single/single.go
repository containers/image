// Package single implements a basic credentials helper with a single username and secret.
package single

import (
	"github.com/docker/docker-credential-helpers/credentials"
)

// AuthStore is a basic credentials store that holds a single entry.
type AuthStore credentials.Credentials

// Get retrieves credentials from the store.
func (s *AuthStore) Get(serverURL string) (string, string, error) {
	if s == nil || serverURL != s.ServerURL {
		return "", "", credentials.NewErrCredentialsNotFound()
	}

	return s.Username, s.Secret, nil
}

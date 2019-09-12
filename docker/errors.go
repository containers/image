package docker

import (
	"errors"
	"net/http"

	perrors "github.com/pkg/errors"
)

var (
	// ErrV1NotSupported is returned when we're trying to talk to a
	// docker V1 registry.
	ErrV1NotSupported = errors.New("can't talk to a V1 docker registry")
	// ErrUnauthorizedForCredentials is returned when the status code returned is 401
	ErrUnauthorizedForCredentials = errors.New("unable to retrieve auth token: invalid username/password")
	// ErrTooManyRequests is returned when the status code returned is 429
	ErrTooManyRequests = errors.New("too many request to registry")
)

// httpResponseToError translates the https.Response into an error. It returns
// nil if the response is not considered an error.
func httpResponseToError(res *http.Response) error {
	switch res.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusTooManyRequests:
		return ErrTooManyRequests
	case http.StatusUnauthorized:
		return ErrUnauthorizedForCredentials
	default:
		return perrors.Errorf("invalid status code from registry %d (%s)", res.StatusCode, http.StatusText(res.StatusCode))
	}
}

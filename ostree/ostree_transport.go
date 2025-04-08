package ostree

import (
	"errors"

	"github.com/containers/image/v5/transports"
	"github.com/containers/image/v5/types"
)

// Transport is an ImageTransport for ostree paths.
//
// Deprecated: The ostree: implementation has been removed, and any attempt to use this transport fails.
var Transport = transports.NewStubTransport("ostree")

func init() {
	transports.Register(Transport)
}

// NewReference is no longer implemented, and always fails.
//
// Deprecated: The ostree: implementation has been removed.
func NewReference(image string, repo string) (types.ImageReference, error) {
	return nil, errors.New("The ostree: implementation has been removed.")
}

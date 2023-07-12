//go:build containers_image_ostree
// +build containers_image_ostree

package ostree

import (
	"github.com/containers/image/v5/internal/private"
)

var _ private.ImageDestination = (*ostreeImageDestination)(nil)

package archive

import "github.com/containers/image/v5/internal/private"

var _ private.ImageDestination = (*archiveImageDestination)(nil)

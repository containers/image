package none

import (
	"github.com/containers/image/v5/types"
)

var _ types.BlobInfoCache = &noCache{}

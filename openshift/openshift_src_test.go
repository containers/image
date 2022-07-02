package openshift

import "github.com/containers/image/v5/internal/private"

var _ private.ImageSource = (*openshiftImageSource)(nil)

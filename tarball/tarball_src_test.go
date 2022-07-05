package tarball

import "github.com/containers/image/v5/internal/private"

var _ private.ImageSource = (*tarballImageSource)(nil)

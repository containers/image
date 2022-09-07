package sif

import "github.com/containers/image/v5/internal/private"

var _ private.ImageSource = (*sifImageSource)(nil)

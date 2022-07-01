//go:build containers_image_ostree
// +build containers_image_ostree

package ostree

var _ private.ImageDestination = (*ostreeImageDestination)(nil)

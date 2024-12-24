//go:build !linux || !cgo

package reflink

import (
	"io"
	"os"
)

// Copy attempts to reflink the source to the destination fd.
// If reflinking fails or is unsupported, it falls back to io.Copy().
func Copy(src, dst *os.File) error {
	_, err := io.Copy(dst, src)
	return err
}

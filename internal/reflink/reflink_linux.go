//go:build cgo

package reflink

/*
#include <linux/fs.h>

#ifndef FICLONE
#define FICLONE		_IOW(0x94, 9, int)
#endif
*/
import "C"
import (
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// Copy attempts to reflink the source to the destination fd.
// If reflinking fails or is unsupported, it falls back to io.Copy().
func Copy(src, dst *os.File) error {
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, dst.Fd(), C.FICLONE, src.Fd())
	if errno == 0 {
		return nil
	}

	_, err := io.Copy(dst, src)
	return err
}

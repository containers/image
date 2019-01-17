// +build !containers_image_ostree_stub,!mac_stub

package ostree

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	selinux "github.com/opencontainers/selinux/go-selinux"
	"github.com/pkg/errors"
)

// #cgo pkg-config: glib-2.0 libselinux
// #include <glib.h>
// #include <stdlib.h>
// #include <selinux/selinux.h>
// #include <selinux/label.h>
import "C"

type mandatoryAccessControl interface {
	Close()
	ChangeLabels(root string, fullpath string, fileMode os.FileMode) error
}

type macSelinux struct {
	handler *C.struct_selabel_handle
}

func createMac() (mandatoryAccessControl, error) {
	if os.Getuid() == 0 && selinux.GetEnabled() {
		selinuxHnd, err := C.selabel_open(C.SELABEL_CTX_FILE, nil, 0)
		if selinuxHnd == nil {
			return nil, errors.Wrapf(err, "cannot open the SELinux DB")
		}
		return &macSelinux{selinuxHnd}, nil
	}

	return &macStub{}, nil
}

func (m macSelinux) Close() {
	C.selabel_close(m.handler)
}

func (m macSelinux) ChangeLabels(root string, fullpath string, fileMode os.FileMode) error {
	relPath, err := filepath.Rel(root, fullpath)
	if err != nil {
		return err
	}
	// Handle /exports/hostfs as a special case.  Files under this directory are copied to the host,
	// thus we benefit from maintaining the same SELinux label they would have on the host as we could
	// use hard links instead of copying the files.
	relPath = fmt.Sprintf("/%s", strings.TrimPrefix(relPath, "exports/hostfs/"))

	relPathC := C.CString(relPath)
	defer C.free(unsafe.Pointer(relPathC))
	var context *C.char

	res, err := C.selabel_lookup_raw(m.handler, &context, relPathC, C.int(fileMode&os.ModePerm))
	if int(res) < 0 && err != syscall.ENOENT {
		return errors.Wrapf(err, "cannot selabel_lookup_raw %s", relPath)
	}
	if int(res) == 0 {
		defer C.freecon(context)
		fullpathC := C.CString(fullpath)
		defer C.free(unsafe.Pointer(fullpathC))
		res, err = C.lsetfilecon_raw(fullpathC, context)
		if int(res) < 0 {
			return errors.Wrapf(err, "cannot setfilecon_raw %s", fullpath)
		}
	}

	return nil
}

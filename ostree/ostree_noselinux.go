// +build !containers_image_ostree_stub,mac_stub

package ostree

import "os"

type mandatoryAccessControl interface {
	Close()
	ChangeLabels(root string, fullpath string, fileMode os.FileMode) error
}

func createMac() (mandatoryAccessControl, error) {
	return &macStub{}, nil
}

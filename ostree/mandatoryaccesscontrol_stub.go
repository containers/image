package ostree

import "os"

type macStub struct{}

func (m macStub) Close() {}

func (m macStub) ChangeLabels(root string, fullpath string, fileMode os.FileMode) error {
	return nil
}

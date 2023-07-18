package tmpdir

import (
	"os"
	"strings"
	"testing"

	"github.com/containers/image/v5/types"
	"github.com/stretchr/testify/assert"
)

func TestCreateBigFileTemp(t *testing.T) {
	f, err := CreateBigFileTemp(nil, "")
	assert.NoError(t, err)
	f.Close()
	os.Remove(f.Name())

	f, err = CreateBigFileTemp(nil, "foobar")
	assert.NoError(t, err)
	f.Close()
	assert.True(t, strings.Contains(f.Name(), prefix+"foobar"))
	os.Remove(f.Name())

	var sys types.SystemContext
	sys.BigFilesTemporaryDir = "/tmp"
	f, err = CreateBigFileTemp(&sys, "foobar1")
	assert.NoError(t, err)
	f.Close()
	assert.True(t, strings.Contains(f.Name(), "/tmp/"+prefix+"foobar1"))
	os.Remove(f.Name())

	sys.BigFilesTemporaryDir = "/tmp/bogus"
	_, err = CreateBigFileTemp(&sys, "foobar1")
	assert.Error(t, err)

}

func TestMkDirBigFileTemp(t *testing.T) {
	d, err := MkDirBigFileTemp(nil, "foobar")
	assert.NoError(t, err)
	assert.True(t, strings.Contains(d, prefix+"foobar"))
	os.RemoveAll(d)

	var sys types.SystemContext
	sys.BigFilesTemporaryDir = "/tmp"
	d, err = MkDirBigFileTemp(&sys, "foobar1")
	assert.NoError(t, err)
	assert.True(t, strings.Contains(d, "/tmp/"+prefix+"foobar1"))
	os.RemoveAll(d)

	sys.BigFilesTemporaryDir = "/tmp/bogus"
	_, err = MkDirBigFileTemp(&sys, "foobar1")
	assert.Error(t, err)
}

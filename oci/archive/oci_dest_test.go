package archive

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/containers/image/v5/internal/private"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ private.ImageDestination = (*ociArchiveImageDestination)(nil)

func TestTarDirectory(t *testing.T) {
	srcDir := t.TempDir()
	err := os.WriteFile(filepath.Join(srcDir, "regular"), []byte("contents"), 0o600)
	require.NoError(t, err)

	dest := filepath.Join(t.TempDir(), "file.tar")
	err = tarDirectory(srcDir, dest, nil)
	require.NoError(t, err)

	f, err := os.Open(dest)
	require.NoError(t, err)
	defer f.Close()
	reader := tar.NewReader(f)
	numItems := 0
	for {
		hdr, err := reader.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		// Test that the header does not expose data about the local account
		assert.Equal(t, 0, hdr.Uid)
		assert.Equal(t, 0, hdr.Gid)
		assert.Empty(t, hdr.Uname)
		assert.Empty(t, hdr.Gname)
		numItems++
	}
	assert.Equal(t, 1, numItems)
}

package streamdigest

import (
	"io"
	"os"
	"testing"

	"github.com/containers/image/v5/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeBlobInfo(t *testing.T) {
	inputInfo := types.BlobInfo{Digest: "", Size: -1}
	fixtureFname := "fixtures/Hello.uncompressed"
	fixtureInfo := types.BlobInfo{Digest: "sha256:185f8db32271fe25f561a6fc938b2e264306ec304eda518007d1764826381969", Size: 5}
	fixtureBytes := []byte("Hello")

	// open fixture
	stream, err := os.Open(fixtureFname)
	require.NoError(t, err, fixtureFname)
	defer stream.Close()

	// fill in Digest and Size for inputInfo
	streamCopy, cleanup, err := ComputeBlobInfo(nil, stream, &inputInfo)
	require.NoError(t, err)
	defer cleanup()

	// ensure inputInfo has been filled in with Digest and Size of fixture
	assert.Equal(t, inputInfo, fixtureInfo)

	// ensure streamCopy is the same as fixture
	b, err := io.ReadAll(streamCopy)
	require.NoError(t, err)
	assert.Equal(t, b, fixtureBytes)
}

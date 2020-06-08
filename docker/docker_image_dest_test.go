package docker

import (
	"testing"

	"github.com/containers/image/v5/types"
	"github.com/stretchr/testify/assert"
)

func TestDockerKnownLayerCompression(t *testing.T) {
	dest := dockerImageDestination{}
	info := types.BlobInfo{MediaType: "this is not a known media type"}
	assert.Equal(t, types.PreserveOriginal, dest.DesiredBlobCompression(info))
}

package tarfile

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/pkg/blobinfocache/memory"
	"github.com/containers/image/v5/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSourcePrepareLayerData(t *testing.T) {
	// Just a smoke test to verify prepareLayerData does not crash on missing data
	for _, c := range []struct {
		config     string
		shouldFail bool
	}{
		{`{}`, true},             // No RootFS entry: can fail, shouldn’t crash
		{`{"rootfs":{}}`, false}, // Useless no-layer configuration
	} {
		cache := memory.New()
		var tarfileBuffer bytes.Buffer
		ctx := context.Background()

		writer := NewWriter(&tarfileBuffer)
		dest := NewDestination(nil, writer, nil)
		// No layers
		configInfo, err := dest.PutBlob(ctx, strings.NewReader(c.config),
			types.BlobInfo{Size: -1}, cache, true)
		require.NoError(t, err, c.config)
		manifest, err := manifest.Schema2FromComponents(
			manifest.Schema2Descriptor{
				MediaType: manifest.DockerV2Schema2ConfigMediaType,
				Size:      configInfo.Size,
				Digest:    configInfo.Digest,
			}, []manifest.Schema2Descriptor{}).Serialize()
		require.NoError(t, err, c.config)
		err = dest.PutManifest(ctx, manifest, nil)
		require.NoError(t, err, c.config)
		err = writer.Close()
		require.NoError(t, err, c.config)

		reader, err := NewReaderFromStream(nil, &tarfileBuffer)
		require.NoError(t, err, c.config)
		src := NewSource(reader, true, nil, -1)
		require.NoError(t, err, c.config)
		defer src.Close()
		configStream, _, err := src.GetBlob(ctx, types.BlobInfo{
			Digest: configInfo.Digest,
			Size:   -1,
		}, cache)
		if !c.shouldFail {
			require.NoError(t, err, c.config)
			config2, err := io.ReadAll(configStream)
			require.NoError(t, err, c.config)
			assert.Equal(t, []byte(c.config), config2, c.config)
		} else {
			assert.Error(t, err, c.config)
		}
	}
}

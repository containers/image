package tarfile

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/pkg/blobinfocache/memory"
	"github.com/containers/image/v5/types"
	digest "github.com/opencontainers/go-digest"
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
		dest := NewDestination(nil, writer, "transport name", nil, nil)
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
		src := NewSource(reader, true, "transport name", nil, -1)
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

func TestSourceGetBlobSymlinkLayerSizeMatchesBytesReturned(t *testing.T) {
	ctx := context.Background()
	cache := memory.New()

	layerBytes := []byte("not empty")
	diffID := digest.FromBytes(layerBytes)
	configBytes := []byte(`{"rootfs":{"type":"layers","diff_ids":["` + diffID.String() + `"]}}`)

	manifestBytes, err := json.Marshal([]ManifestItem{
		{
			Config: "config.json",
			Layers: []string{"layer-link.tar"},
		},
	})
	require.NoError(t, err)

	var tarfileBuffer bytes.Buffer
	tw := tar.NewWriter(&tarfileBuffer)
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name: "manifest.json",
		Mode: 0o644,
		Size: int64(len(manifestBytes)),
	}))
	_, err = tw.Write(manifestBytes)
	require.NoError(t, err)

	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name: "config.json",
		Mode: 0o644,
		Size: int64(len(configBytes)),
	}))
	_, err = tw.Write(configBytes)
	require.NoError(t, err)

	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name: "layer.tar",
		Mode: 0o644,
		Size: int64(len(layerBytes)),
	}))
	_, err = tw.Write(layerBytes)
	require.NoError(t, err)

	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name:     "layer-link.tar",
		Typeflag: tar.TypeSymlink,
		Linkname: "layer.tar",
		Mode:     0o777,
	}))
	require.NoError(t, tw.Close())

	reader, err := NewReaderFromStream(nil, &tarfileBuffer)
	require.NoError(t, err)
	src := NewSource(reader, true, "transport name", nil, -1)
	t.Cleanup(func() { _ = src.Close() })

	layerStream, reportedSize, err := src.GetBlob(ctx, types.BlobInfo{
		Digest: diffID,
		Size:   -1,
	}, cache)
	require.NoError(t, err)
	defer layerStream.Close()

	readBytes, err := io.ReadAll(layerStream)
	require.NoError(t, err)
	assert.Equal(t, layerBytes, readBytes)
	assert.Equal(t, int64(len(layerBytes)), reportedSize)
}

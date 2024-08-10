package blobcache

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cp "github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/directory"
	"github.com/containers/image/v5/internal/image"
	"github.com/containers/image/v5/internal/private"
	"github.com/containers/image/v5/pkg/blobinfocache/none"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/types"
	"github.com/containers/storage/pkg/archive"
	digest "github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
)

var (
	_ types.ImageReference     = &BlobCache{}
	_ types.ImageSource        = &blobCacheSource{}
	_ private.ImageSource      = (*blobCacheSource)(nil)
	_ types.ImageDestination   = &blobCacheDestination{}
	_ private.ImageDestination = (*blobCacheDestination)(nil)
)

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Verbose() {
		logrus.SetLevel(logrus.DebugLevel)
	}
	os.Exit(m.Run())
}

// Create a layer containing a single file with the specified name (and its
// name as its contents), compressed using the specified compression type, and
// return the .
func makeLayer(filename string, repeat int, compression archive.Compression) ([]byte, digest.Digest, error) {
	var compressed, uncompressed bytes.Buffer
	layer, err := archive.Generate(filename, strings.Repeat(filename, repeat))
	if err != nil {
		return nil, "", err
	}
	writer, err := archive.CompressStream(&compressed, compression)
	if err != nil {
		return nil, "", err
	}
	reader := io.TeeReader(layer, &uncompressed)
	_, err = io.Copy(writer, reader)
	writer.Close()
	if err != nil {
		return nil, "", err
	}
	return compressed.Bytes(), digest.FromBytes(uncompressed.Bytes()), nil
}

func TestBlobCache(t *testing.T) {
	cacheDir := t.TempDir()

	systemContext := types.SystemContext{BlobInfoCacheDir: "/dev/null/this/does/not/exist"}

	for _, repeat := range []int{1, 10000} {
		for _, desiredCompression := range []types.LayerCompression{types.PreserveOriginal, types.Compress, types.Decompress} {
			for _, layerCompression := range []archive.Compression{archive.Uncompressed, archive.Gzip} {
				// Create a layer with the specified layerCompression.
				blobBytes, diffID, err := makeLayer(fmt.Sprintf("layer-content-%d", int(layerCompression)), repeat, layerCompression)
				if err != nil {
					t.Fatalf("error making layer: %v", err)
				}
				blobInfo := types.BlobInfo{
					Digest: digest.FromBytes(blobBytes),
					Size:   int64(len(blobBytes)),
				}
				// Create a configuration that includes the diffID for the layer and not much else.
				config := v1.Image{
					RootFS: v1.RootFS{
						Type:    "layers",
						DiffIDs: []digest.Digest{diffID},
					},
				}
				configBytes, err := json.Marshal(&config)
				if err != nil {
					t.Fatalf("error encoding image configuration: %v", err)
				}
				configInfo := types.BlobInfo{
					Digest: digest.FromBytes(configBytes),
					Size:   int64(len(configBytes)),
				}
				// Create a manifest that uses this configuration and layer.
				manifest := v1.Manifest{
					Versioned: specs.Versioned{
						SchemaVersion: 2,
					},
					MediaType: v1.MediaTypeImageManifest,
					Config: v1.Descriptor{
						MediaType: v1.MediaTypeImageConfig,
						Digest:    configInfo.Digest,
						Size:      configInfo.Size,
					},
					Layers: []v1.Descriptor{{
						MediaType: v1.MediaTypeImageLayer,
						Digest:    blobInfo.Digest,
						Size:      blobInfo.Size,
					}},
				}
				manifestBytes, err := json.Marshal(&manifest)
				if err != nil {
					t.Fatalf("error encoding image manifest: %v", err)
				}
				// Write this image to a "dir" destination with blob caching using this directory.
				srcdir := t.TempDir()
				srcRef, err := directory.NewReference(srcdir)
				if err != nil {
					t.Fatalf("error creating source image name reference for %q: %v", srcdir, err)
				}
				cachedSrcRef, err := NewBlobCache(srcRef, cacheDir, desiredCompression)
				if err != nil {
					t.Fatalf("failed to wrap reference in cache: %v", err)
				}
				destImage, err := cachedSrcRef.NewImageDestination(context.TODO(), nil)
				if err != nil {
					t.Fatalf("error opening source image for writing: %v", err)
				}
				_, err = destImage.PutBlob(context.TODO(), bytes.NewReader(blobBytes), blobInfo, none.NoCache, false)
				if err != nil {
					t.Fatalf("error writing layer blob to source image: %v", err)
				}
				_, err = destImage.PutBlob(context.TODO(), bytes.NewReader(configBytes), configInfo, none.NoCache, true)
				if err != nil {
					t.Fatalf("error writing config blob to source image: %v", err)
				}
				srcImage, err := srcRef.NewImageSource(context.TODO(), &systemContext)
				if err != nil {
					t.Fatalf("error opening source image: %v", err)
				}
				defer func() {
					err := srcImage.Close()
					if err != nil {
						t.Fatalf("error closing source image: %v", err)
					}
				}()
				err = destImage.PutManifest(context.TODO(), manifestBytes, nil)
				if err != nil {
					t.Fatalf("error writing manifest to source image: %v", err)
				}
				err = destImage.Commit(context.TODO(), image.UnparsedInstance(srcImage, nil))
				if err != nil {
					t.Fatalf("error committing source image: %v", err)
				}
				if err = destImage.Close(); err != nil {
					t.Fatalf("error closing source image: %v", err)
				}
				// Check that the cache was populated.
				cachedNames, err := os.ReadDir(cacheDir)
				if err != nil {
					t.Fatal(err)
				}
				// Expect a layer blob, a config blob, and the manifest.
				expected := 3
				if layerCompression != archive.Uncompressed {
					// Expect a compressed blob, an uncompressed blob, notes for each about the other, a config blob, and the manifest.
					expected = 6
				}
				if len(cachedNames) != expected {
					t.Fatalf("expected %d items in cache directory %q, got %d: %v", expected, cacheDir, len(cachedNames), cachedNames)
				}
				// Check that the blobs were all correctly stored.
				for _, de := range cachedNames {
					cachedName := de.Name()
					if digest.Digest(cachedName).Validate() == nil {
						cacheMember := filepath.Join(cacheDir, cachedName)
						cacheMemberBytes, err := os.ReadFile(cacheMember)
						if err != nil {
							t.Fatal(err)
						}
						if digest.FromBytes(cacheMemberBytes).String() != cachedName {
							t.Fatalf("cache member %q was stored incorrectly!", cacheMember)
						}
					}
				}
				// Clear out anything in the source directory that probably isn't a manifest, so that we'll
				// have to depend on the cached copies of some of the blobs.
				srcNames, err := os.ReadDir(srcdir)
				if err != nil {
					t.Fatal(err)
				}
				for _, de := range srcNames {
					name := de.Name()
					if !strings.HasPrefix(name, "manifest") {
						os.Remove(filepath.Join(srcdir, name))
					}
				}
				// Now that we've deleted some of the contents, try to copy from the source image
				// to a second image.  It should fail because the source is missing some blobs.
				destdir := t.TempDir()
				destRef, err := directory.NewReference(destdir)
				if err != nil {
					t.Fatalf("error creating destination image reference for %q: %v", destdir, err)
				}
				options := cp.Options{
					SourceCtx:      &systemContext,
					DestinationCtx: &systemContext,
				}
				policyContext, err := signature.NewPolicyContext(&signature.Policy{
					Default: []signature.PolicyRequirement{signature.NewPRInsecureAcceptAnything()},
				})
				if err != nil {
					t.Fatalf("error creating signature policy context: %v", err)
				}
				_, err = cp.Image(context.TODO(), policyContext, destRef, srcRef, &options)
				if err == nil {
					t.Fatalf("expected an error copying the image, but got success")
				} else {
					if errors.Is(err, fs.ErrNotExist) {
						t.Logf("ok: got expected does-not-exist error copying the image with blobs missing: %v", err)
					} else {
						t.Logf("got an error copying the image with missing blobs, but not sure which error: %v", err)
					}
				}
				_, err = cp.Image(context.TODO(), policyContext, destRef, cachedSrcRef, &options)
				if err != nil {
					t.Fatalf("unexpected error copying the image using the cache: %v", err)
				}
				if err = cachedSrcRef.ClearCache(); err != nil {
					t.Fatalf("error clearing cache: %v", err)
				}
			}
		}
	}
}

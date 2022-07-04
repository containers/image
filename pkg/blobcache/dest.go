package blobcache

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/transports"
	"github.com/containers/image/v5/types"
	"github.com/containers/storage/pkg/archive"
	"github.com/containers/storage/pkg/ioutils"
	digest "github.com/opencontainers/go-digest"
	perrors "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type blobCacheDestination struct {
	reference   *BlobCache
	destination types.ImageDestination
}

func (b *BlobCache) NewImageDestination(ctx context.Context, sys *types.SystemContext) (types.ImageDestination, error) {
	dest, err := b.reference.NewImageDestination(ctx, sys)
	if err != nil {
		return nil, perrors.Wrapf(err, "error creating new image destination %q", transports.ImageName(b.reference))
	}
	logrus.Debugf("starting to write to image %q using blob cache in %q", transports.ImageName(b.reference), b.directory)
	return &blobCacheDestination{reference: b, destination: dest}, nil
}

func (d *blobCacheDestination) Reference() types.ImageReference {
	return d.reference
}

func (d *blobCacheDestination) Close() error {
	logrus.Debugf("finished writing to image %q using blob cache", transports.ImageName(d.reference))
	return d.destination.Close()
}

func (d *blobCacheDestination) SupportedManifestMIMETypes() []string {
	return d.destination.SupportedManifestMIMETypes()
}

func (d *blobCacheDestination) SupportsSignatures(ctx context.Context) error {
	return d.destination.SupportsSignatures(ctx)
}

func (d *blobCacheDestination) DesiredLayerCompression() types.LayerCompression {
	return d.destination.DesiredLayerCompression()
}

func (d *blobCacheDestination) AcceptsForeignLayerURLs() bool {
	return d.destination.AcceptsForeignLayerURLs()
}

func (d *blobCacheDestination) MustMatchRuntimeOS() bool {
	return d.destination.MustMatchRuntimeOS()
}

func (d *blobCacheDestination) IgnoresEmbeddedDockerReference() bool {
	return d.destination.IgnoresEmbeddedDockerReference()
}

// Decompress and save the contents of the decompressReader stream into the passed-in temporary
// file.  If we successfully save all of the data, rename the file to match the digest of the data,
// and make notes about the relationship between the file that holds a copy of the compressed data
// and this new file.
func saveStream(wg *sync.WaitGroup, decompressReader io.ReadCloser, tempFile *os.File, compressedFilename string, compressedDigest digest.Digest, isConfig bool, alternateDigest *digest.Digest) {
	defer wg.Done()
	// Decompress from and digest the reading end of that pipe.
	decompressed, err3 := archive.DecompressStream(decompressReader)
	digester := digest.Canonical.Digester()
	if err3 == nil {
		// Read the decompressed data through the filter over the pipe, blocking until the
		// writing end is closed.
		_, err3 = io.Copy(io.MultiWriter(tempFile, digester.Hash()), decompressed)
	} else {
		// Drain the pipe to keep from stalling the PutBlob() thread.
		if _, err := io.Copy(io.Discard, decompressReader); err != nil {
			logrus.Debugf("error draining the pipe: %v", err)
		}
	}
	decompressReader.Close()
	decompressed.Close()
	tempFile.Close()
	// Determine the name that we should give to the uncompressed copy of the blob.
	decompressedFilename := filepath.Join(filepath.Dir(tempFile.Name()), makeFilename(digester.Digest(), isConfig))
	if err3 == nil {
		// Rename the temporary file.
		if err3 = os.Rename(tempFile.Name(), decompressedFilename); err3 != nil {
			logrus.Debugf("error renaming new decompressed copy of blob %q into place at %q: %v", digester.Digest().String(), decompressedFilename, err3)
			// Remove the temporary file.
			if err3 = os.Remove(tempFile.Name()); err3 != nil {
				logrus.Debugf("error cleaning up temporary file %q for decompressed copy of blob %q: %v", tempFile.Name(), compressedDigest.String(), err3)
			}
		} else {
			*alternateDigest = digester.Digest()
			// Note the relationship between the two files.
			if err3 = ioutils.AtomicWriteFile(decompressedFilename+compressedNote, []byte(compressedDigest.String()), 0600); err3 != nil {
				logrus.Debugf("error noting that the compressed version of %q is %q: %v", digester.Digest().String(), compressedDigest.String(), err3)
			}
			if err3 = ioutils.AtomicWriteFile(compressedFilename+decompressedNote, []byte(digester.Digest().String()), 0600); err3 != nil {
				logrus.Debugf("error noting that the decompressed version of %q is %q: %v", compressedDigest.String(), digester.Digest().String(), err3)
			}
		}
	} else {
		// Remove the temporary file.
		if err3 = os.Remove(tempFile.Name()); err3 != nil {
			logrus.Debugf("error cleaning up temporary file %q for decompressed copy of blob %q: %v", tempFile.Name(), compressedDigest.String(), err3)
		}
	}
}

func (d *blobCacheDestination) HasThreadSafePutBlob() bool {
	return d.destination.HasThreadSafePutBlob()
}

func (d *blobCacheDestination) PutBlob(ctx context.Context, stream io.Reader, inputInfo types.BlobInfo, cache types.BlobInfoCache, isConfig bool) (types.BlobInfo, error) {
	var tempfile *os.File
	var err error
	var n int
	var alternateDigest digest.Digest
	var closer io.Closer
	wg := new(sync.WaitGroup)
	needToWait := false
	compression := archive.Uncompressed
	if inputInfo.Digest != "" {
		filename := filepath.Join(d.reference.directory, makeFilename(inputInfo.Digest, isConfig))
		tempfile, err = os.CreateTemp(d.reference.directory, makeFilename(inputInfo.Digest, isConfig))
		if err == nil {
			stream = io.TeeReader(stream, tempfile)
			defer func() {
				if err == nil {
					if err = os.Rename(tempfile.Name(), filename); err != nil {
						if err2 := os.Remove(tempfile.Name()); err2 != nil {
							logrus.Debugf("error cleaning up temporary file %q for blob %q: %v", tempfile.Name(), inputInfo.Digest.String(), err2)
						}
						err = perrors.Wrapf(err, "error renaming new layer for blob %q into place at %q", inputInfo.Digest.String(), filename)
					}
				} else {
					if err2 := os.Remove(tempfile.Name()); err2 != nil {
						logrus.Debugf("error cleaning up temporary file %q for blob %q: %v", tempfile.Name(), inputInfo.Digest.String(), err2)
					}
				}
				tempfile.Close()
			}()
		} else {
			logrus.Debugf("error while creating a temporary file under %q to hold blob %q: %v", d.reference.directory, inputInfo.Digest.String(), err)
		}
		if !isConfig {
			initial := make([]byte, 8)
			n, err = stream.Read(initial)
			if n > 0 {
				// Build a Reader that will still return the bytes that we just
				// read, for PutBlob()'s sake.
				stream = io.MultiReader(bytes.NewReader(initial[:n]), stream)
				if n >= len(initial) {
					compression = archive.DetectCompression(initial[:n])
				}
				if compression == archive.Gzip {
					// The stream is compressed, so create a file which we'll
					// use to store a decompressed copy.
					decompressedTemp, err2 := os.CreateTemp(d.reference.directory, makeFilename(inputInfo.Digest, isConfig))
					if err2 != nil {
						logrus.Debugf("error while creating a temporary file under %q to hold decompressed blob %q: %v", d.reference.directory, inputInfo.Digest.String(), err2)
					} else {
						// Write a copy of the compressed data to a pipe,
						// closing the writing end of the pipe after
						// PutBlob() returns.
						decompressReader, decompressWriter := io.Pipe()
						closer = decompressWriter
						stream = io.TeeReader(stream, decompressWriter)
						// Let saveStream() close the reading end and handle the temporary file.
						wg.Add(1)
						needToWait = true
						go saveStream(wg, decompressReader, decompressedTemp, filename, inputInfo.Digest, isConfig, &alternateDigest)
					}
				}
			}
		}
	}
	newBlobInfo, err := d.destination.PutBlob(ctx, stream, inputInfo, cache, isConfig)
	if closer != nil {
		closer.Close()
	}
	if needToWait {
		wg.Wait()
	}
	if err != nil {
		return newBlobInfo, perrors.Wrapf(err, "error storing blob to image destination for cache %q", transports.ImageName(d.reference))
	}
	if alternateDigest.Validate() == nil {
		logrus.Debugf("added blob %q (also %q) to the cache at %q", inputInfo.Digest.String(), alternateDigest.String(), d.reference.directory)
	} else {
		logrus.Debugf("added blob %q to the cache at %q", inputInfo.Digest.String(), d.reference.directory)
	}
	return newBlobInfo, nil
}

func (d *blobCacheDestination) TryReusingBlob(ctx context.Context, info types.BlobInfo, cache types.BlobInfoCache, canSubstitute bool) (bool, types.BlobInfo, error) {
	present, reusedInfo, err := d.destination.TryReusingBlob(ctx, info, cache, canSubstitute)
	if err != nil || present {
		return present, reusedInfo, err
	}

	for _, isConfig := range []bool{false, true} {
		filename := filepath.Join(d.reference.directory, makeFilename(info.Digest, isConfig))
		f, err := os.Open(filename)
		if err == nil {
			defer f.Close()
			uploadedInfo, err := d.destination.PutBlob(ctx, f, info, cache, isConfig)
			if err != nil {
				return false, types.BlobInfo{}, err
			}
			return true, uploadedInfo, nil
		}
	}

	return false, types.BlobInfo{}, nil
}

func (d *blobCacheDestination) PutManifest(ctx context.Context, manifestBytes []byte, instanceDigest *digest.Digest) error {
	manifestDigest, err := manifest.Digest(manifestBytes)
	if err != nil {
		logrus.Warnf("error digesting manifest %q: %v", string(manifestBytes), err)
	} else {
		filename := filepath.Join(d.reference.directory, makeFilename(manifestDigest, false))
		if err = ioutils.AtomicWriteFile(filename, manifestBytes, 0600); err != nil {
			logrus.Warnf("error saving manifest as %q: %v", filename, err)
		}
	}
	return d.destination.PutManifest(ctx, manifestBytes, instanceDigest)
}

func (d *blobCacheDestination) PutSignatures(ctx context.Context, signatures [][]byte, instanceDigest *digest.Digest) error {
	return d.destination.PutSignatures(ctx, signatures, instanceDigest)
}

func (d *blobCacheDestination) Commit(ctx context.Context, unparsedToplevel types.UnparsedImage) error {
	return d.destination.Commit(ctx, unparsedToplevel)
}

package mocks

import (
	"context"
	"io"

	"github.com/containers/image/v5/types"
	"github.com/opencontainers/go-digest"
)

// ForbiddenImageSource is used when we don't expect the ImageSource to be used in our tests.
type ForbiddenImageSource struct{}

// Reference is a mock that panics.
func (f ForbiddenImageSource) Reference() types.ImageReference {
	panic("Unexpected call to a mock function")
}

// Close is a mock that panics.
func (f ForbiddenImageSource) Close() error {
	panic("Unexpected call to a mock function")
}

// GetManifest is a mock that panics.
func (f ForbiddenImageSource) GetManifest(context.Context, *digest.Digest) ([]byte, string, error) {
	panic("Unexpected call to a mock function")
}

// GetBlob is a mock that panics.
func (f ForbiddenImageSource) GetBlob(context.Context, types.BlobInfo, types.BlobInfoCache) (io.ReadCloser, int64, error) {
	panic("Unexpected call to a mock function")
}

// HasThreadSafeGetBlob is a mock that panics.
func (f ForbiddenImageSource) HasThreadSafeGetBlob() bool {
	panic("Unexpected call to a mock function")
}

// GetSignatures is a mock that panics.
func (f ForbiddenImageSource) GetSignatures(context.Context, *digest.Digest) ([][]byte, error) {
	panic("Unexpected call to a mock function")
}

// LayerInfosForCopy is a mock that panics.
func (f ForbiddenImageSource) LayerInfosForCopy(context.Context, *digest.Digest) ([]types.BlobInfo, error) {
	panic("Unexpected call to a mock function")
}

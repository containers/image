package mocks

import (
	"context"

	"github.com/containers/image/v5/types"
)

// ForbiddenUnparsedImage is used when we don't expect the UnparsedImage to be used in our tests.
type ForbiddenUnparsedImage struct{}

// Reference is a mock that panics.
func (ref ForbiddenUnparsedImage) Reference() types.ImageReference {
	panic("unexpected call to a mock function")
}

// Manifest is a mock that panics.
func (ref ForbiddenUnparsedImage) Manifest(ctx context.Context) ([]byte, string, error) {
	panic("unexpected call to a mock function")
}

// Signatures is a mock that panics.
func (ref ForbiddenUnparsedImage) Signatures(context.Context) ([][]byte, error) {
	panic("unexpected call to a mock function")
}

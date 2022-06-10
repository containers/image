package mocks

import (
	"context"

	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/types"
)

// ForbiddenImageReference is used when we don’t expect the ImageReference to be used in our tests.
type ForbiddenImageReference struct{}

// Transport is a mock that panics.
func (ref ForbiddenImageReference) Transport() types.ImageTransport {
	panic("unexpected call to a mock function")
}

// StringWithinTransport is a mock that panics.
func (ref ForbiddenImageReference) StringWithinTransport() string {
	panic("unexpected call to a mock function")
}

// DockerReference is a mock that panics.
func (ref ForbiddenImageReference) DockerReference() reference.Named {
	panic("unexpected call to a mock function")
}

// PolicyConfigurationIdentity is a mock that panics.
func (ref ForbiddenImageReference) PolicyConfigurationIdentity() string {
	panic("unexpected call to a mock function")
}

// PolicyConfigurationNamespaces is a mock that panics.
func (ref ForbiddenImageReference) PolicyConfigurationNamespaces() []string {
	panic("unexpected call to a mock function")
}

// NewImage is a mock that panics.
func (ref ForbiddenImageReference) NewImage(ctx context.Context, sys *types.SystemContext) (types.ImageCloser, error) {
	panic("unexpected call to a mock function")
}

// NewImageSource is a mock that panics.
func (ref ForbiddenImageReference) NewImageSource(ctx context.Context, sys *types.SystemContext) (types.ImageSource, error) {
	panic("unexpected call to a mock function")
}

// NewImageDestination is a mock that panics.
func (ref ForbiddenImageReference) NewImageDestination(ctx context.Context, sys *types.SystemContext) (types.ImageDestination, error) {
	panic("unexpected call to a mock function")
}

// DeleteImage is a mock that panics.
func (ref ForbiddenImageReference) DeleteImage(ctx context.Context, sys *types.SystemContext) error {
	panic("unexpected call to a mock function")
}

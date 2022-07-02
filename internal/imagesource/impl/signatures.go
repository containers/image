package impl

import (
	"context"

	"github.com/opencontainers/go-digest"
)

// NoSignatures implements GetSignatures() that returns nothing.
type NoSignatures struct{}

// GetSignatures returns the image's signatures.  It may use a remote (= slow) service.
// If instanceDigest is not nil, it contains a digest of the specific manifest instance to retrieve signatures for
// (when the primary manifest is a manifest list); this never happens if the primary manifest is not a manifest list
// (e.g. if the source never returns manifest lists).
func (stub NoSignatures) GetSignatures(ctx context.Context, instanceDigest *digest.Digest) ([][]byte, error) {
	return nil, nil
}

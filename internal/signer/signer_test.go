package signer

import (
	"context"
	"errors"
	"testing"

	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/internal/signature"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSignerImplementation is a SignerImplementation used only for tests.
type mockSignerImplementation struct {
	progressMessage   func() string
	signImageManifest func(ctx context.Context, m []byte, dockerReference reference.Named) (signature.Signature, error)
	close             func() error
}

func (ms *mockSignerImplementation) Close() error {
	return ms.close()
}

func (ms *mockSignerImplementation) ProgressMessage() string {
	return ms.progressMessage()
}

func (ms *mockSignerImplementation) SignImageManifest(ctx context.Context, m []byte, dockerReference reference.Named) (signature.Signature, error) {
	return ms.signImageManifest(ctx, m, dockerReference)
}

func TestNewSigner(t *testing.T) {
	closeError := errors.New("unique error")

	si := mockSignerImplementation{
		// Other functions are nil, so this ensures they are not called.
		close: func() error { return closeError },
	}
	s := NewSigner(&si)
	// Verify SignerImplementation methods are not visible even to determined callers
	_, visible := any(s).(SignerImplementation)
	assert.False(t, visible)
	err := s.Close()
	assert.Equal(t, closeError, err)
}

func TestProgressMessage(t *testing.T) {
	si := mockSignerImplementation{
		// Other functions are nil, so this ensures they are not called.
		close: func() error { return nil },
	}
	s := NewSigner(&si)
	defer s.Close()

	const testMessage = "some unique string"
	si.progressMessage = func() string {
		return testMessage
	}
	message := ProgressMessage(s)
	assert.Equal(t, testMessage, message)
}

func TestSignImageManifest(t *testing.T) {
	si := mockSignerImplementation{
		// Other functions are nil, so this ensures they are not called.
		close: func() error { return nil },
	}
	s := NewSigner(&si)
	defer s.Close()

	testManifest := []byte("some manifest")
	testDR, err := reference.ParseNormalizedNamed("busybox")
	require.NoError(t, err)
	type contextKeyType struct{}
	testContext := context.WithValue(context.Background(), contextKeyType{}, "make this context unique")
	testSig := signature.SigstoreFromComponents(signature.SigstoreSignatureMIMEType, []byte("payload"), nil)
	testErr := errors.New("some unique error")
	si.signImageManifest = func(ctx context.Context, m []byte, dockerReference reference.Named) (signature.Signature, error) {
		assert.Equal(t, testContext, ctx)
		assert.Equal(t, testManifest, m)
		assert.Equal(t, testDR, dockerReference)
		return testSig, testErr
	}
	sig, err := SignImageManifest(testContext, s, testManifest, testDR)
	assert.Equal(t, testSig, sig)
	assert.Equal(t, testErr, err)
}

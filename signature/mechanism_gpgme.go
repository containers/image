//go:build !containers_image_openpgp
// +build !containers_image_openpgp

package signature

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/containers/image/v5/signature/internal"
	"github.com/proglottis/gpgme"
)

// A GPG/OpenPGP signing mechanism, implemented using gpgme.
type gpgmeSigningMechanism struct {
	ctx          *gpgme.Context
	ephemeralDir string // If not "", a directory to be removed on Close()
}

// newGPGSigningMechanismInDirectory returns a new GPG/OpenPGP signing mechanism, using optionalDir if not empty.
// The caller must call .Close() on the returned SigningMechanism.
func newGPGSigningMechanismInDirectory(optionalDir string) (signingMechanismWithPassphrase, error) {
	ctx, err := newGPGMEContext(optionalDir)
	if err != nil {
		return nil, err
	}
	return &gpgmeSigningMechanism{
		ctx:          ctx,
		ephemeralDir: "",
	}, nil
}

// newEphemeralGPGSigningMechanism returns a new GPG/OpenPGP signing mechanism which
// recognizes _only_ public keys from the supplied blobs, and returns the identities
// of these keys.
// The caller must call .Close() on the returned SigningMechanism.
func newEphemeralGPGSigningMechanism(blobs [][]byte) (signingMechanismWithPassphrase, []string, error) {
	dir, err := os.MkdirTemp("", "containers-ephemeral-gpg-")
	if err != nil {
		return nil, nil, err
	}
	removeDir := true
	defer func() {
		if removeDir {
			os.RemoveAll(dir)
		}
	}()
	ctx, err := newGPGMEContext(dir)
	if err != nil {
		return nil, nil, err
	}
	mech := &gpgmeSigningMechanism{
		ctx:          ctx,
		ephemeralDir: dir,
	}
	// Use a 60-second timeout for key import operations to prevent indefinite hanging
	// This helps mitigate file descriptor leaks and prevents the process from hanging
	const importTimeout = 60 * time.Second

	keyIdentities := []string{}
	for _, blob := range blobs {
		ki, err := mech.importKeysFromBytes(blob, importTimeout)
		if err != nil {
			return nil, nil, err
		}
		keyIdentities = append(keyIdentities, ki...)
	}

	removeDir = false
	return mech, keyIdentities, nil
}

// newGPGMEContext returns a new *gpgme.Context, using optionalDir if not empty.
func newGPGMEContext(optionalDir string) (*gpgme.Context, error) {
	ctx, err := gpgme.New()
	if err != nil {
		return nil, err
	}
	if err = ctx.SetProtocol(gpgme.ProtocolOpenPGP); err != nil {
		return nil, err
	}
	if optionalDir != "" {
		err := ctx.SetEngineInfo(gpgme.ProtocolOpenPGP, "", optionalDir)
		if err != nil {
			return nil, err
		}
	}
	ctx.SetArmor(false)
	ctx.SetTextMode(false)
	return ctx, nil
}

func (m *gpgmeSigningMechanism) Close() error {
	if m.ephemeralDir != "" {
		os.RemoveAll(m.ephemeralDir) // Ignore an error, if any
	}
	return nil
}

// cancellableReader is a reader that can be cancelled to interrupt long-running operations.
// When cancellation is detected, it returns an error to stop GPGME from reading further,
// which helps prevent file descriptor leaks and allows cleanup.
type cancellableReader struct {
	reader    io.Reader
	cancelCh  <-chan struct{}
	cancelled bool
}

func (r *cancellableReader) Read(p []byte) (int, error) {
	// Check for cancellation before each read
	select {
	case <-r.cancelCh:
		r.cancelled = true
		return 0, fmt.Errorf("GPG key import operation timed out or was cancelled")
	default:
		// Continue reading
	}

	n, err := r.reader.Read(p)

	// Check for cancellation after read (in case cancellation happened during read)
	if !r.cancelled {
		select {
		case <-r.cancelCh:
			r.cancelled = true
			// Return error immediately to stop further reads
			if n > 0 {
				// We read some data, but cancellation occurred - return error
				return n, fmt.Errorf("GPG key import operation timed out or was cancelled")
			}
			return 0, fmt.Errorf("GPG key import operation timed out or was cancelled")
		default:
		}
	}

	return n, err
}

// importKeysFromBytes imports public keys from the supplied blob and returns their identities.
// The blob is assumed to have an appropriate format (the caller is expected to know which one).
// NOTE: This may modify long-term state (e.g. key storage in a directory underlying the mechanism);
// but we do not make this public, it can only be used through newEphemeralGPGSigningMechanism.
// Uses a cancellable reader with timeout to prevent indefinite hanging and file descriptor leaks.
func (m *gpgmeSigningMechanism) importKeysFromBytes(blob []byte, timeout time.Duration) ([]string, error) {
	// Create a context with timeout for the import operation
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Create a cancellable reader that wraps the blob data
	// This allows us to interrupt the operation if it takes too long
	reader := &cancellableReader{
		reader:   bytes.NewReader(blob),
		cancelCh: ctx.Done(),
	}

	inputData, err := gpgme.NewDataReader(reader)
	if err != nil {
		return nil, err
	}

	// Wrap the Import call in a timeout to prevent indefinite hanging.
	// File descriptor cleanup happens when:
	// 1. Timeout occurs -> cancellable reader's cancelCh is closed
	// 2. GPGME tries to read more data -> reader.Read() returns error
	// 3. Import call fails with the reader error -> GPGME cleans up file descriptors
	// 4. Goroutine exits -> resources are released
	type importResult struct {
		res *gpgme.ImportResult
		err error
	}
	resultCh := make(chan importResult, 1)

	go func() {
		res, err := m.ctx.Import(inputData)
		// Send result (buffered channel, so this won't block if caller timed out)
		resultCh <- importResult{res: res, err: err}
	}()

	select {
	case <-ctx.Done():
		// Timeout occurred - return error immediately.
		// File descriptor cleanup: The cancellable reader will return an error
		// when GPGME tries to read more data (if it reads incrementally).
		// This error will cause Import to fail, triggering GPGME's cleanup.
		// The goroutine will continue running until Import fails or completes,
		// but file descriptors should be cleaned up when the reader error propagates.
		return nil, fmt.Errorf("GPG key import operation timed out after %s", timeout)
	case result := <-resultCh:
		// Import completed - check if it was cancelled during the operation
		if reader.cancelled {
			return nil, fmt.Errorf("GPG key import operation timed out")
		}
		if result.err != nil {
			return nil, result.err
		}

		keyIdentities := []string{}
		for _, i := range result.res.Imports {
			if i.Result == nil {
				keyIdentities = append(keyIdentities, i.Fingerprint)
			}
		}
		return keyIdentities, nil
	}
}

// SupportsSigning returns nil if the mechanism supports signing, or a SigningNotSupportedError.
func (m *gpgmeSigningMechanism) SupportsSigning() error {
	return nil
}

// Sign creates a (non-detached) signature of input using keyIdentity and passphrase.
// Fails with a SigningNotSupportedError if the mechanism does not support signing.
func (m *gpgmeSigningMechanism) SignWithPassphrase(input []byte, keyIdentity string, passphrase string) ([]byte, error) {
	key, err := m.ctx.GetKey(keyIdentity, true)
	if err != nil {
		return nil, err
	}
	inputData, err := gpgme.NewDataBytes(input)
	if err != nil {
		return nil, err
	}
	var sigBuffer bytes.Buffer
	sigData, err := gpgme.NewDataWriter(&sigBuffer)
	if err != nil {
		return nil, err
	}

	if passphrase != "" {
		// Callback to write the passphrase to the specified file descriptor.
		callback := func(uidHint string, prevWasBad bool, gpgmeFD *os.File) error {
			if prevWasBad {
				return errors.New("bad passphrase")
			}
			_, err := gpgmeFD.WriteString(passphrase + "\n")
			return err
		}
		if err := m.ctx.SetCallback(callback); err != nil {
			return nil, fmt.Errorf("setting gpgme passphrase callback: %w", err)
		}

		// Loopback mode will use the callback instead of prompting the user.
		if err := m.ctx.SetPinEntryMode(gpgme.PinEntryLoopback); err != nil {
			return nil, fmt.Errorf("setting gpgme pinentry mode: %w", err)
		}
	}

	if err = m.ctx.Sign([]*gpgme.Key{key}, inputData, sigData, gpgme.SigModeNormal); err != nil {
		return nil, err
	}
	return sigBuffer.Bytes(), nil
}

// Sign creates a (non-detached) signature of input using keyIdentity.
// Fails with a SigningNotSupportedError if the mechanism does not support signing.
func (m *gpgmeSigningMechanism) Sign(input []byte, keyIdentity string) ([]byte, error) {
	return m.SignWithPassphrase(input, keyIdentity, "")
}

// Verify parses unverifiedSignature and returns the content and the signer's identity
func (m *gpgmeSigningMechanism) Verify(unverifiedSignature []byte) (contents []byte, keyIdentity string, err error) {
	signedBuffer := bytes.Buffer{}
	signedData, err := gpgme.NewDataWriter(&signedBuffer)
	if err != nil {
		return nil, "", err
	}
	unverifiedSignatureData, err := gpgme.NewDataBytes(unverifiedSignature)
	if err != nil {
		return nil, "", err
	}
	_, sigs, err := m.ctx.Verify(unverifiedSignatureData, nil, signedData)
	if err != nil {
		return nil, "", err
	}
	if len(sigs) != 1 {
		return nil, "", internal.NewInvalidSignatureError(fmt.Sprintf("Unexpected GPG signature count %d", len(sigs)))
	}
	sig := sigs[0]
	// This is sig.Summary == gpgme.SigSumValid except for key trust, which we handle ourselves
	if sig.Status != nil || sig.Validity == gpgme.ValidityNever || sig.ValidityReason != nil || sig.WrongKeyUsage {
		// FIXME: Better error reporting eventually
		return nil, "", internal.NewInvalidSignatureError(fmt.Sprintf("Invalid GPG signature: %#v", sig))
	}
	return signedBuffer.Bytes(), sig.Fingerprint, nil
}

// UntrustedSignatureContents returns UNTRUSTED contents of the signature WITHOUT ANY VERIFICATION,
// along with a short identifier of the key used for signing.
// WARNING: The short key identifier (which corresponds to "Key ID" for OpenPGP keys)
// is NOT the same as a "key identity" used in other calls to this interface, and
// the values may have no recognizable relationship if the public key is not available.
func (m *gpgmeSigningMechanism) UntrustedSignatureContents(untrustedSignature []byte) (untrustedContents []byte, shortKeyIdentifier string, err error) {
	return gpgUntrustedSignatureContents(untrustedSignature)
}

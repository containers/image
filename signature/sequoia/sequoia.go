//go:build containers_image_sequoia

package sequoia

// #cgo CFLAGS: -I. -DGO_SEQUOIA_ENABLE_DLOPEN=1
// #include "gosequoia.h"
// #include <dlfcn.h>
import "C"

import (
	"errors"
	"unsafe"
)

type SigningMechanism struct {
	mechanism *C.SequoiaMechanism
}

func NewMechanismFromDirectory(
	dir string,
) (*SigningMechanism, error) {
	var cerr *C.SequoiaError
	cMechanism := C.go_sequoia_mechanism_new_from_directory(C.CString(dir), &cerr)
	if cMechanism == nil {
		defer C.go_sequoia_error_free(cerr)
		return nil, errors.New(C.GoString(cerr.message))
	}
	return &SigningMechanism{
		mechanism: cMechanism,
	}, nil
}

func NewEphemeralMechanism() (*SigningMechanism, error) {
	var cerr *C.SequoiaError
	cMechanism := C.go_sequoia_mechanism_new_ephemeral(&cerr)
	if cMechanism == nil {
		defer C.go_sequoia_error_free(cerr)
		return nil, errors.New(C.GoString(cerr.message))
	}
	return &SigningMechanism{
		mechanism: cMechanism,
	}, nil
}

func (m *SigningMechanism) SignWithPassphrase(
	input []byte,
	keyIdentity string,
	passphrase string,
) ([]byte, error) {
	var cerr *C.SequoiaError
	var cPassphrase *C.char
	if passphrase == "" {
		cPassphrase = nil
	} else {
		cPassphrase = C.CString(passphrase)
	}
	sig := C.go_sequoia_sign(
		m.mechanism,
		C.CString(keyIdentity),
		cPassphrase,
		base(input), C.size_t(len(input)),
		&cerr,
	)
	if sig == nil {
		defer C.go_sequoia_error_free(cerr)
		return nil, errors.New(C.GoString(cerr.message))
	}
	defer C.go_sequoia_signature_free(sig)
	var size C.size_t
	cData := C.go_sequoia_signature_get_data(sig, &size)
	return C.GoBytes(unsafe.Pointer(cData), C.int(size)), nil
}

func (m *SigningMechanism) Sign(
	input []byte,
	keyIdentity string,
) ([]byte, error) {
	return m.SignWithPassphrase(input, keyIdentity, "")
}

func (m *SigningMechanism) Verify(
	unverifiedSignature []byte,
) (contents []byte, keyIdentity string, err error) {
	var cerr *C.SequoiaError
	result := C.go_sequoia_verify(
		m.mechanism,
		base(unverifiedSignature), C.size_t(len(unverifiedSignature)),
		&cerr,
	)
	if result == nil {
		defer C.go_sequoia_error_free(cerr)
		return nil, "", errors.New(C.GoString(cerr.message))
	}
	defer C.go_sequoia_verification_result_free(result)
	var size C.size_t
	cContent := C.go_sequoia_verification_result_get_content(result, &size)
	contents = C.GoBytes(unsafe.Pointer(cContent), C.int(size))
	cSigner := C.go_sequoia_verification_result_get_signer(result)
	keyIdentity = C.GoString(cSigner)
	return
}

func (m *SigningMechanism) ImportKeys(blob []byte) ([]string, error) {
	var cerr *C.SequoiaError
	result := C.go_sequoia_import_keys(
		m.mechanism,
		base(blob),
		C.size_t(len(blob)),
		&cerr,
	)
	if result == nil {
		defer C.go_sequoia_error_free(cerr)
		return nil, errors.New(C.GoString(cerr.message))
	}

	keyIdentities := []string{}
	count := C.go_sequoia_import_result_get_count(result)
	for i := 0; i < int(count); i++ {
		cKeyIdentity := C.go_sequoia_import_result_get_content(result, C.size_t(i), &cerr)
		keyIdentities = append(keyIdentities, C.GoString(cKeyIdentity))
	}

	return keyIdentities, nil
}

func (m *SigningMechanism) Close() error {
	return nil
}

func (m *SigningMechanism) SupportsSigning() error {
	return nil
}

// base returns the address of the underlying array in b,
// being careful not to panic when b has zero length.
func base(b []byte) *C.uchar {
	if len(b) == 0 {
		return nil
	}
	return (*C.uchar)(unsafe.Pointer(&b[0]))
}

func Init() error {
	if C.go_sequoia_ensure_library(C.CString("libimage_sequoia.so.0"),
		C.RTLD_NOW|C.RTLD_GLOBAL) < 0 {
		return errors.New("unable to load libimage_sequoia.so.0")
	}
	return nil
}

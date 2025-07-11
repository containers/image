//go:build containers_image_sequoia

package sequoia

// #cgo CFLAGS: -I. -DGO_SEQUOIA_ENABLE_DLOPEN=1
// #include "gosequoia.h"
// #include <dlfcn.h>
// #include <limits.h>
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
	cDir := C.CString(dir)
	defer C.free(unsafe.Pointer(cDir))
	cMechanism := C.go_sequoia_mechanism_new_from_directory(cDir, &cerr)
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
		defer C.free(unsafe.Pointer(cPassphrase))
	}
	cKeyIdentity := C.CString(keyIdentity)
	defer C.free(unsafe.Pointer(cKeyIdentity))
	sig := C.go_sequoia_sign(
		m.mechanism,
		cKeyIdentity,
		cPassphrase,
		(*C.uchar)(unsafe.Pointer(unsafe.SliceData(input))),
		C.size_t(len(input)),
		&cerr,
	)
	if sig == nil {
		defer C.go_sequoia_error_free(cerr)
		return nil, errors.New(C.GoString(cerr.message))
	}
	defer C.go_sequoia_signature_free(sig)
	var size C.size_t
	cData := C.go_sequoia_signature_get_data(sig, &size)
	if size > C.size_t(C.INT_MAX) {
		return nil, errors.New("overflow")
	}
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
		(*C.uchar)(unsafe.Pointer(unsafe.SliceData(unverifiedSignature))),
		C.size_t(len(unverifiedSignature)),
		&cerr,
	)
	if result == nil {
		defer C.go_sequoia_error_free(cerr)
		return nil, "", errors.New(C.GoString(cerr.message))
	}
	defer C.go_sequoia_verification_result_free(result)
	var size C.size_t
	cContent := C.go_sequoia_verification_result_get_content(result, &size)
	if size > C.size_t(C.INT_MAX) {
		return nil, "", errors.New("overflow")
	}
	contents = C.GoBytes(unsafe.Pointer(cContent), C.int(size))
	cSigner := C.go_sequoia_verification_result_get_signer(result)
	keyIdentity = C.GoString(cSigner)
	return contents, keyIdentity, nil
}

func (m *SigningMechanism) ImportKeys(blob []byte) ([]string, error) {
	var cerr *C.SequoiaError
	result := C.go_sequoia_import_keys(
		m.mechanism,
		(*C.uchar)(unsafe.Pointer(unsafe.SliceData(blob))),
		C.size_t(len(blob)),
		&cerr,
	)
	if result == nil {
		defer C.go_sequoia_error_free(cerr)
		return nil, errors.New(C.GoString(cerr.message))
	}
	defer C.go_sequoia_import_result_free(result)

	keyIdentities := []string{}
	count := C.go_sequoia_import_result_get_count(result)
	for i := C.size_t(0); i < count; i++ {
		cKeyIdentity := C.go_sequoia_import_result_get_content(result, i, &cerr)
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

func Init() error {
	if C.go_sequoia_ensure_library(C.CString("libpodman_sequoia.so.0"),
		C.RTLD_NOW|C.RTLD_GLOBAL) < 0 {
		return errors.New("unable to load libpodman_sequoia.so.0")
	}
	return nil
}

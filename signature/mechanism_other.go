//go:build !containers_image_openpgp && !cgo
// +build !containers_image_openpgp,!cgo

package signature

// CgoIsDisabled indicates as a compiler error that this package requires cgo.
//
// You may enable the containers_image_openpgp build tag but beware that this
// implementation is not actively maintained and is considered insecure.
var CgoIsDisabled = ContainersImageSignatureRequiresCgo

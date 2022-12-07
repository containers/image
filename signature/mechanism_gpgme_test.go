//go:build !containers_image_openpgp && !containers_image_disable_signing && cgo
// +build !containers_image_openpgp,!containers_image_disable_signing,cgo

package signature

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGPGMESigningMechanismClose(t *testing.T) {
	// Closing an ephemeral mechanism removes the directory.
	// (The non-ephemeral case is tested in the common TestGPGSigningMechanismClose)
	mech, _, err := NewEphemeralGPGSigningMechanism([]byte{})
	require.NoError(t, err)
	gpgMech, ok := mech.(*gpgmeSigningMechanism)
	require.True(t, ok)
	dir := gpgMech.ephemeralDir
	assert.NotEmpty(t, dir)
	_, err = os.Lstat(dir)
	require.NoError(t, err)
	err = mech.Close()
	assert.NoError(t, err)
	_, err = os.Lstat(dir)
	require.Error(t, err)
	assert.True(t, os.IsNotExist(err))
}

func TestGPGMESigningMechanismSupportsSigning(t *testing.T) {
	mech, _, err := NewEphemeralGPGSigningMechanism([]byte{})
	require.NoError(t, err)
	defer mech.Close()
	err = mech.SupportsSigning()
	assert.NoError(t, err)
}

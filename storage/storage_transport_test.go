package storage

import (
	"fmt"
	"testing"

	"github.com/containers/image/docker/reference"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	sha256digestHex = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
)

func TestTransportName(t *testing.T) {
	assert.Equal(t, "containers-storage", Transport.Name())
}

func TestTransportParseStoreReference(t *testing.T) {
	for _, c := range []struct{ input, expectedRef, expectedID string }{
		{"", "", ""}, // Empty input
		// Handling of the store prefix
		// FIXME? Should we be silently discarding input like this?
		{"[unterminated", "", ""},                                    // Unterminated store specifier
		{"[garbage]busybox", "docker.io/library/busybox:latest", ""}, // Store specifier is ignored

		{"UPPERCASEISINVALID", "", ""},                                                     // Invalid single-component name
		{"sha256:" + sha256digestHex, "", sha256digestHex},                                 // Valid single-component ID
		{sha256digestHex, "", sha256digestHex},                                             // Valid single-component ID, implicit digest.Canonical
		{"sha256:ab", "docker.io/library/sha256:ab", ""},                                   // Valid single-component name (ParseIDOrReference accepts digest prefixes as names!) (FIXME? is this desirable?)
		{"busybox", "docker.io/library/busybox:latest", ""},                                // Valid single-component name, implicit tag
		{"busybox:notlatest", "docker.io/library/busybox:notlatest", ""},                   // Valid single-component name, explicit tag
		{"docker.io/library/busybox:notlatest", "docker.io/library/busybox:notlatest", ""}, // Valid single-component name, everything explicit

		{"UPPERCASEISINVALID@" + sha256digestHex, "", ""},                                                                  // Invalid name in name@ID
		{"busybox@ab", "", ""},                                                                                             // Invalid ID in name@ID
		{"busybox@", "", ""},                                                                                               // Empty ID in name@ID
		{"busybox@sha256:" + sha256digestHex, "", ""},                                                                      // This (in a digested docker/docker reference format) is also invalid
		{"@" + sha256digestHex, "", sha256digestHex},                                                                       // Valid two-component name, with ID only
		{"busybox@" + sha256digestHex, "docker.io/library/busybox:latest", sha256digestHex},                                // Valid two-component name, implicit tag
		{"busybox:notlatest@" + sha256digestHex, "docker.io/library/busybox:notlatest", sha256digestHex},                   // Valid two-component name, explicit tag
		{"docker.io/library/busybox:notlatest@" + sha256digestHex, "docker.io/library/busybox:notlatest", sha256digestHex}, // Valid two-component name, everything explicit
	} {
		ref, err := Transport.ParseStoreReference(Transport.(*storageTransport).store, c.input)
		if c.expectedRef == "" && c.expectedID == "" {
			assert.Error(t, err, c.input)
		} else {
			require.NoError(t, err, c.input)
			storageRef, ok := ref.(*storageReference)
			require.True(t, ok, c.input)
			assert.Equal(t, *(Transport.(*storageTransport)), storageRef.transport, c.input)
			assert.Equal(t, c.expectedRef, storageRef.reference, c.input)
			assert.Equal(t, c.expectedID, storageRef.id, c.input)
			if c.expectedRef == "" {
				assert.Nil(t, storageRef.name, c.input)
			} else {
				dockerRef, err := reference.ParseNamed(c.expectedRef)
				require.NoError(t, err)
				require.NotNil(t, storageRef.name, c.input)
				assert.Equal(t, dockerRef.String(), storageRef.name.String())
			}
		}
	}
}

func TestTransportParseReference(t *testing.T) {
	store := newStore(t)
	driver := store.GetGraphDriverName()
	root := store.GetGraphRoot()

	for _, c := range []struct{ prefix, expectedDriver, expectedRoot string }{
		{"", driver, root},          // Implicit store location prefix
		{"[unterminated", "", ""},   // Unterminated store specifier
		{"[]", "", ""},              // Empty store specifier
		{"[relative/path]", "", ""}, // Non-absolute graph root path
		//{"[" + root + "suffix1]", driver, root + "suffix1"}, // A valid root path FIXME: this currently fails
		{"[" + driver + "@relative/path]", "", ""},      // Non-absolute graph root path
		{"[thisisunknown@" + root + "suffix2]", "", ""}, // Unknown graph driver
		//{"[" + driver + "@" + root + "suffix3]", driver, root + "suffix3"}, // A valid root@graph  FIXME: this currently fails
	} {
		ref, err := Transport.ParseReference(c.prefix + "busybox")
		if c.expectedDriver == "" {
			assert.Error(t, err, c.prefix)
		} else {
			require.NoError(t, err, c.prefix)
			storageRef, ok := ref.(*storageReference)
			require.True(t, ok, c.prefix)
			assert.Equal(t, c.expectedDriver, storageRef.transport.store.GetGraphDriverName(), c.prefix)
			assert.Equal(t, c.expectedRoot, storageRef.transport.store.GetGraphRoot(), c.prefix)
		}
	}
}

func TestTransportValidatePolicyConfigurationScope(t *testing.T) {
	store := newStore(t)
	driver := store.GetGraphDriverName()
	root := store.GetGraphRoot()
	storeSpec := fmt.Sprintf("[%s@%s]", driver, root) // As computed in PolicyConfigurationNamespaces

	// Valid inputs
	for _, scope := range []string{
		"[" + root + "suffix1]",                                              // driverlessStoreSpec in PolicyConfigurationNamespaces
		"[" + driver + "@" + root + "suffix3]",                               // storeSpec
		storeSpec + "sha256:ab",                                              // Valid single-component name (ParseIDOrReference accepts digest prefixes as names!) (FIXME? is this desirable?)
		storeSpec + "busybox",                                                // Valid single-component name, implicit tag; NOTE that this non-canonical form would be interpreted as a scope for host busybox
		storeSpec + "busybox:notlatest",                                      // Valid single-component name, explicit tag; NOTE that this non-canonical form would be interpreted as a scope for host busybox
		storeSpec + "docker.io/library/busybox:notlatest",                    // Valid single-component name, everything explicit
		storeSpec + "busybox@" + sha256digestHex,                             // Valid two-component name, implicit tag; NOTE that this non-canonical form would be interpreted as a scope for host busybox (and never match)
		storeSpec + "busybox:notlatest@" + sha256digestHex,                   // Valid two-component name, explicit tag; NOTE that this non-canonical form would be interpreted as a scope for host busybox (and never match)
		storeSpec + "docker.io/library/busybox:notlatest@" + sha256digestHex, // Valid two-component name, everything explicit
	} {
		err := Transport.ValidatePolicyConfigurationScope(scope)
		assert.NoError(t, err, scope)
	}

	// Invalid inputs
	for _, scope := range []string{
		// "busybox", // Unprefixed reference; FIXME: This can't actually be matched by a storageReference.PolicyConfiguration{Identity,Namespaces}, so it should be rejected
		"[unterminated",                  // Unterminated store specifier
		"[]",                             // Empty store specifier
		"[relative/path]",                // Non-absolute graph root path
		"[" + driver + "@relative/path]", // Non-absolute graph root path
		// "[thisisunknown@" + root + "suffix2]", // Unknown graph driver FIXME? Should this be detected?
		storeSpec + "sha256:" + sha256digestHex, // Valid single-component ID, but ID-only
		storeSpec + sha256digestHex,             // Valid single-component ID, implicit digest.Canonical, but ID-only
		storeSpec + "@",                         // A completely two-component name
		storeSpec + "@" + sha256digestHex,       // Valid two-component name, but ID-only

		storeSpec + "UPPERCASEISINVALID",                    // Invalid single-component name
		storeSpec + "UPPERCASEISINVALID@" + sha256digestHex, // Invalid name in name@ID
		storeSpec + "busybox@ab",                            // Invalid ID in name@ID
		storeSpec + "busybox@",                              // Empty ID in name@ID
		// storeSpec + "busybox@sha256:" + sha256digestHex,     // This (in a digested docker/docker reference format) is also invalid; FIXME: This can't actually be matched by a storageReference.PolicyConfigurationIdentity, so it should be rejected
	} {
		err := Transport.ValidatePolicyConfigurationScope(scope)
		assert.Error(t, err, scope)
	}
}

package signatures

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/containers/image/docker/reference"
	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTagMappingBucketKey(t *testing.T) {
	// Success
	out, err := tagMappingBucketKey(fxConfigDigest, fxNamedTaggedReference)
	require.NoError(t, err)
	assert.Equal(t, fxTagMappingKey, out)

	// NUL in input is refused.
	_, err = tagMappingBucketKey(digest.Digest("X\x00X"), fxNamedTaggedReference)
	assert.Error(t, err)
	// NUL in ref untested, the reference package prohibits such values; we would have to intentionally
	// create our own implementation of the interface just for this test.
}

func TestResolveManifestDigest(t *testing.T) {
	tx, cleanup := readTx(t, fxDBPath)
	defer cleanup()

	// No reference provided. FIXME: Should this return a manifest if there is only one known digest?
	md, err := resolveManifestDigest(tx, fxConfigDigest, nil)
	require.NoError(t, err)
	assert.Equal(t, digest.Digest(""), md)

	// Config+manifest digests specified explicitly:
	// Success
	md, err = resolveManifestDigest(tx, fxConfigDigest, digestReference(t, fxManifestDigest))
	require.NoError(t, err)
	assert.Equal(t, digest.Digest(fxManifestDigest), md)
	// Manifest digest specified explicitly, even if not stored in the database
	md, err = resolveManifestDigest(tx, fxConfigDigest, digestReference(t, fxConfigDigest))
	require.NoError(t, err)
	assert.Equal(t, digest.Digest(fxConfigDigest), md)
	// Manifest digest specified explicitly, even if config digest is not found
	md, err = resolveManifestDigest(tx, fxConfigDigest+"this does not match", digestReference(t, fxManifestDigest))
	require.NoError(t, err)
	assert.Equal(t, digest.Digest(fxManifestDigest), md)

	// Config digest + NamedTagged:
	// Success, input in minimal form
	named := reference.TagNameOnly(referenceNamed(t, "busybox")) // Note that an untagged reference is ignored!
	md, err = resolveManifestDigest(tx, fxConfigDigest, named)
	require.NoError(t, err)
	assert.Equal(t, digest.Digest(fxManifestDigest), md)
	// Success, input in canonical form
	ref := referenceNamed(t, fxNamedTaggedReference.String())
	md, err = resolveManifestDigest(tx, fxConfigDigest, ref)
	require.NoError(t, err)
	assert.Equal(t, digest.Digest(fxManifestDigest), md)
	// Error computing tagMappingKey
	_, err = resolveManifestDigest(tx, digest.Digest("X\x00X"), ref) // Invalid config digest
	assert.Error(t, err)
	// Name not found in database
	ref = referenceNamed(t, fxNamedTaggedReference.String()+"thisdoesnotexist")
	md, err = resolveManifestDigest(tx, fxConfigDigest, ref)
	require.NoError(t, err)
	assert.Equal(t, digest.Digest(""), md)

	// tagMappingBucket does not exist at all.
	emptyDB := emptyBoltDB(t)
	defer os.Remove(emptyDB)
	tx, cleanup = readTx(t, emptyDB)
	defer cleanup()
	md, err = resolveManifestDigest(tx, fxConfigDigest, named)
	require.NoError(t, nil)
	assert.Equal(t, digest.Digest(""), md)

	// Invalid digest in database
	tmpFile, err := ioutil.TempFile("", "resolveManifestDigest-empty")
	require.NoError(t, err)
	func() { // A scope for defer
		tx, commit := rwTx(t, tmpFile.Name())
		defer commit()
		b, err := tx.CreateBucketIfNotExists(tagMappingBucket)
		require.NoError(t, err)
		nt, ok := named.(reference.NamedTagged)
		require.True(t, ok)
		tagMappingKey, err := tagMappingBucketKey(fxConfigDigest, nt)
		require.NoError(t, err)
		b.Put(tagMappingKey, []byte("X\x00X"))
	}()
	tx, cleanup = readTx(t, tmpFile.Name())
	defer cleanup()
	md, err = resolveManifestDigest(tx, fxConfigDigest, named)
	assert.Error(t, err)
}

func TestUpdateTagMapping(t *testing.T) {
	tmpFile, err := ioutil.TempFile("", "signatures-test-store")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	// Try things twice to see that updating an existing database works.
	// Success, no reference
	for i := 0; i < 2; i++ {
		func() { // A scope for defer
			tx, commit := rwTx(t, tmpFile.Name())
			defer commit()
			err := updateTagMapping(tx, fxConfigDigest, nil, fxManifestDigest)
			assert.NoError(t, err)
		}()
		// This is trivially true because we didn't write anything, but test for completeness.
		func() { // A scope for defer
			tx, cleanup := readTx(t, tmpFile.Name())
			defer cleanup()
			md, err := resolveManifestDigest(tx, fxConfigDigest, digestReference(t, fxManifestDigest))
			require.NoError(t, err)
			assert.Equal(t, digest.Digest(fxManifestDigest), md)
		}()
	}
	// Success, with a reference
	ref := reference.TagNameOnly(referenceNamed(t, "busybox")) // Note that an untagged reference is ignored!
	nt, ok := ref.(reference.NamedTagged)
	require.True(t, ok)
	for i := 0; i < 2; i++ {
		func() { // A scope for defer
			tx, commit := rwTx(t, tmpFile.Name())
			defer commit()
			err := updateTagMapping(tx, fxConfigDigest, nt, fxManifestDigest)
			require.NoError(t, err)
		}()
		func() { // A scope for defer
			tx, cleanup := readTx(t, tmpFile.Name())
			defer cleanup()
			md, err := resolveManifestDigest(tx, fxConfigDigest, nt)
			require.NoError(t, err)
			assert.Equal(t, digest.Digest(fxManifestDigest), md)
		}()
	}

	// Invalid config digest
	func() { // A scope for defer
		tx, commit := rwTx(t, tmpFile.Name())
		defer commit()
		err := updateTagMapping(tx, digest.Digest("X\x00X"), nt, fxManifestDigest)
		assert.Error(t, err)
	}()

	// Failure updating the database is untested.
}

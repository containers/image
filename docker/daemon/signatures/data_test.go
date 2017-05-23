package signatures

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"
	"unsafe"

	bolt "github.com/etcd-io/bbolt"
	digest "github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDataBucketKey(t *testing.T) {
	// Success
	out, err := dataBucketKey(fxConfigDigest, fxManifestDigest)
	require.NoError(t, err)
	assert.Equal(t, fxDataKey, out)

	// NUL in input is refused.
	_, err = dataBucketKey(digest.Digest("X\x00X"), fxManifestDigest)
	assert.Error(t, err)
	_, err = dataBucketKey(fxConfigDigest, digest.Digest("X\x00X"))
	assert.Error(t, err)
}

func TestCopyBytes(t *testing.T) {
	input := []byte("Test data")
	copy := copyBytes(input)
	assert.Equal(t, input, copy) // The contents are the same
	// The underlying memory is distinct.  Cast to uintptr because assert.NotEqual would
	// otherwise compare the pointed-to values, not the pointers.
	assert.NotEqual(t, uintptr(unsafe.Pointer(&input[0])), uintptr(unsafe.Pointer(&copy[0])))
}

// readFxDataBucket returns a *tx.Bucket for the specified path and fxDataKey, and a cleanup handler
func readFxDataBucket(t *testing.T, path string) (*bolt.Bucket, func()) {
	tx, cleanup := readTx(t, path)
	b := tx.Bucket(dataBucket)
	require.NotNil(t, b)
	b = b.Bucket(fxDataKey)
	require.NotNil(t, b)
	return b, cleanup
}

// rwFxDataBucket returns a *bolt.Bucket for the specified path and fxDataKey, and a commit callback.
func rwFxDataBucket(t *testing.T, path string) (*bolt.Bucket, func()) {
	tx, commit := rwTx(t, path)

	b, err := tx.CreateBucketIfNotExists(dataBucket)
	require.NoError(t, err)
	b, err = b.CreateBucketIfNotExists(fxDataKey)
	require.NoError(t, err)
	return b, commit
}

func TestReadManifest(t *testing.T) {
	// Success
	b, cleanup := readFxDataBucket(t, fxDBPath)
	defer cleanup()
	m := readManifest(b)
	assert.Equal(t, fxManifestContents, m)

	// Manifest does not exist in bucket
	// (This should not really happen, writing the manifest and creating the bucket is a single
	// transaction.)
	testWithEmptyFxDataBucket(t, func(path string) {
		b, cleanup := readFxDataBucket(t, path)
		defer cleanup()
		m := readManifest(b)
		assert.Nil(t, m)
	})

	// Failures reading from the database are untested.
}

func TestWriteManifest(t *testing.T) {
	tmpFile, err := ioutil.TempFile("", "signatures-test-writer")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	// Success
	// Try things twice to see that reusing an existing bucket works.
	for i := 0; i < 2; i++ {
		func() { // A scope for defer
			b, commit := rwFxDataBucket(t, tmpFile.Name())
			defer commit()
			err = writeManifest(b, fxManifestContents)
			assert.NoError(t, err)
		}()
		func() { // A scope for defer
			b, cleanup := readFxDataBucket(t, tmpFile.Name())
			defer cleanup()
			m := readManifest(b)
			assert.NotNil(t, m)
			assert.Equal(t, fxManifestContents, m)
		}()
	}

	// Error writing to the database
	b, cleanup := readFxDataBucket(t, tmpFile.Name())
	defer cleanup()
	err = writeManifest(b, []byte("a new manifest to test write failure"))
	assert.Error(t, err)
}

// testWithCorruptSignatures runs fn with a path for a database in which signatures in
// fxDataKey are invalid.
func testWithCorruptSignatures(t *testing.T, fn func(description, path string)) {
	originalFixture, err := ioutil.ReadFile(fxDBPath)
	require.NoError(t, err)
	for _, c := range []struct {
		description string
		fn          func(bucket *bolt.Bucket) error
	}{
		{
			"Invalid signature number", func(bucket *bolt.Bucket) error {
				key := bytes.Join([][]byte{signatureKeyPrefix, []byte("This is not a number")}, []byte{})
				return bucket.Put(key, []byte{})
			},
		},
		{
			"Duplicate signature number", func(bucket *bolt.Bucket) error {
				// 000 is the same index as 0, but the underlying database allows us to store both.
				key := bytes.Join([][]byte{signatureKeyPrefix, []byte("000")}, []byte{})
				return bucket.Put(key, []byte{})
			},
		},
		{
			"Non-consecutive signatures", func(bucket *bolt.Bucket) error {
				// sig2 exists; verify that
				key := bytes.Join([][]byte{signatureKeyPrefix, []byte("2")}, []byte{})
				value := bucket.Get(key)
				require.NotEmpty(t, value)
				// then remove sig1.
				key = bytes.Join([][]byte{signatureKeyPrefix, []byte("1")}, []byte{})
				return bucket.Delete(key)
			},
		},
	} {
		tmpFile, err := ioutil.TempFile("", "signatures-test-corrupt-signatures")
		require.NoError(t, err, c.description)
		defer os.Remove(tmpFile.Name())
		_, err = tmpFile.Write(originalFixture)
		require.NoError(t, err, c.description)

		func() { // A scope for defer
			b, commit := rwFxDataBucket(t, tmpFile.Name())
			defer commit()
			c.fn(b)
		}()

		fn(c.description, tmpFile.Name())
	}
}

func TestReadSignatures(t *testing.T) {
	// Success
	b, cleanup := readFxDataBucket(t, fxDBPath)
	defer cleanup()
	sigs, err := readSignatures(b, fxDataKey)
	require.NoError(t, err)
	assert.Equal(t, fxSignatures, sigs)

	// No signatures exist in the bucket
	testWithEmptyFxDataBucket(t, func(path string) {
		b, cleanup := readFxDataBucket(t, path)
		defer cleanup()
		sigs, err := readSignatures(b, fxDataKey)
		require.NoError(t, err)
		assert.Empty(t, sigs)
	})

	// Corrupt data
	testWithCorruptSignatures(t, func(description, path string) {
		b, cleanup := readFxDataBucket(t, path)
		defer cleanup()
		_, err = readSignatures(b, fxDataKey)
		assert.Error(t, err, description)
	})

	// Failures reading from the database are untested.
}

func TestWriteSignatures(t *testing.T) {
	tmpFile, err := ioutil.TempFile("", "signatures-test-writer")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	// Success
	// Try things twice to see that reusing an existing bucket works and does not store two copies.
	for i := 0; i < 2; i++ {
		func() { // A scope for defer
			b, commit := rwFxDataBucket(t, tmpFile.Name())
			defer commit()
			err = writeSignatures(b, fxDataKey, fxSignatures)
			assert.NoError(t, err)
		}()
		func() { // A scope for defer
			b, cleanup := readFxDataBucket(t, tmpFile.Name())
			defer cleanup()
			sigs, err := readSignatures(b, fxDataKey)
			require.NoError(t, err)
			assert.Equal(t, fxSignatures, sigs)
		}()
	}

	// Adding a new signature to an existing bucket adds it, while preserving the old ones.
	extraTestSignature := []byte("An extra signature")
	func() { // A scope for defer
		b, commit := rwFxDataBucket(t, tmpFile.Name())
		defer commit()
		err = writeSignatures(b, fxDataKey, [][]byte{extraTestSignature})
		assert.NoError(t, err)
	}()
	func() { // A scope for defer
		b, cleanup := readFxDataBucket(t, tmpFile.Name())
		defer cleanup()
		sigs, err := readSignatures(b, fxDataKey)
		require.NoError(t, err)
		assert.Equal(t, append(fxSignatures, extraTestSignature), sigs)
	}()

	// Error reading old signatures
	testWithCorruptSignatures(t, func(description, path string) {
		b, commit := rwFxDataBucket(t, path)
		defer commit()
		err := writeSignatures(b, fxDataKey, [][]byte{extraTestSignature})
		assert.Error(t, err, description)
	})

	// A smoke test for the “no signatures” quick path
	func() { // A scope for defer
		b, commit := rwFxDataBucket(t, tmpFile.Name())
		defer commit()
		err = writeSignatures(b, fxDataKey, [][]byte{})
		assert.NoError(t, err)
	}()

	// Error writing to the database
	b, cleanup := readFxDataBucket(t, tmpFile.Name())
	defer cleanup()
	err = writeSignatures(b, fxDataKey, [][]byte{[]byte("a new signature to test write failure")})
	assert.Error(t, err)
}

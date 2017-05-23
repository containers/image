package signatures

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	"testing"

	bolt "github.com/etcd-io/bbolt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readTx returns a read-only *bolt.Tx for the specified path, and a cleanup handler.
func readTx(t *testing.T, path string) (*bolt.Tx, func()) {
	db, err := bolt.Open(path, 0600, &bolt.Options{ReadOnly: true})
	require.NoError(t, err)
	tx, err := db.Begin(false)
	require.NoError(t, err)
	cleanup := func() {
		err := tx.Rollback()
		require.NoError(t, err)
		err = db.Close()
		require.NoError(t, err)
	}
	return tx, cleanup
}

// rwTx returns a read-write *bolt.Tx for the specified path, and a commit callback.
func rwTx(t *testing.T, path string) (*bolt.Tx, func()) {
	db, err := bolt.Open(path, 0600, nil)
	require.NoError(t, err)
	tx, err := db.Begin(true)
	require.NoError(t, err)
	commit := func() {
		err := tx.Commit()
		require.NoError(t, err)
		err = db.Close()
		require.NoError(t, err)
	}
	return tx, commit
}

// dumpBoltDB returns full contents of database at dbPath as a string suitable for comparisons.
func dumpBoltDB(t *testing.T, dbPath string) string {
	tx, cleanup := readTx(t, dbPath)
	defer cleanup()

	buf := &bytes.Buffer{}
	// We rely on bolt to use, as the documentation says, “byte-sorted order”.
	err := tx.ForEach(func(name []byte, b *bolt.Bucket) error {
		dumpBucket(t, buf, 0, name, b)
		return nil
	})
	require.NoError(t, err)
	return buf.String()
}

// dumpBucket dumps bucket b with name at the specified depth
func dumpBucket(t *testing.T, out io.Writer, depth int, name []byte, bucket *bolt.Bucket) {
	prefix := strings.Repeat("  ", depth)
	fmt.Fprintf(out, "%s[%q]\n", prefix, name)
	depth++

	// We rely on bolt to use, as the documentation says, “byte-sorted order”.
	err := bucket.ForEach(func(k, v []byte) error {
		if v == nil {
			b := bucket.Bucket(k)
			require.NotNil(t, b)
			dumpBucket(t, out, depth, k, b)
		}
		return nil
	})
	require.NoError(t, err)

	// We rely on bolt to use, as the documentation says, “byte-sorted order”.
	err = bucket.ForEach(func(k, v []byte) error {
		if v != nil {
			fmt.Fprintf(out, "%s  %q = %q\n", prefix, k, v)
		}
		return nil
	})
	require.NoError(t, err)
}

// emptyBoltDB returns a path to a temporary file containing an empty database
func emptyBoltDB(t *testing.T) string {
	tmpFile, err := ioutil.TempFile("", "emptyBoltDB")
	require.NoError(t, err)
	db, err := bolt.Open(tmpFile.Name(), 0600, nil)
	require.NoError(t, err)
	err = db.Close()
	require.NoError(t, err)
	return tmpFile.Name()
}

// assertBoltDBMatchesFxDBDump verifies that a database at path matches fxDBDumpPath
func assertBoltDBMatchesFxDBDump(t *testing.T, path string) {
	dump := dumpBoltDB(t, path)
	expectedDump, err := ioutil.ReadFile(fxDBDumpPath)
	require.NoError(t, err)
	assert.Equal(t, string(expectedDump), dump)
}

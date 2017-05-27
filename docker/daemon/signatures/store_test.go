package signatures

import (
	"context"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/containers/image/docker/reference"
	"github.com/containers/image/image"
	"github.com/containers/image/manifest"
	"github.com/containers/image/types"
	bolt "github.com/etcd-io/bbolt"
	digest "github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDockerSignatureDBPath(t *testing.T) {
	const nondefaultPath = "/this/is/not/the/default/signatures.db"
	const variableReference = "$HOME"
	const rootPrefix = "/root/prefix"

	for _, c := range []struct {
		ctx      *types.SystemContext
		expected string
	}{
		// The common case
		{nil, systemDockerSignatureDBPath},
		// There is a context, but it does not override the path.
		{&types.SystemContext{}, systemDockerSignatureDBPath},
		// Path overridden
		{&types.SystemContext{DockerSignatureDBPath: nondefaultPath}, nondefaultPath},
		// Root overridden
		{
			&types.SystemContext{RootForImplicitAbsolutePaths: rootPrefix},
			filepath.Join(rootPrefix, systemDockerSignatureDBPath),
		},
		// Root and path overrides present simultaneously,
		{
			&types.SystemContext{
				RootForImplicitAbsolutePaths: rootPrefix,
				DockerSignatureDBPath:        nondefaultPath,
			},
			nondefaultPath,
		},
		// No environment expansion happens in the overridden paths
		{&types.SystemContext{DockerSignatureDBPath: variableReference}, variableReference},
	} {
		path := dockerSignatureDBPath(c.ctx)
		assert.Equal(t, c.expected, path)
	}
}

func TestStringToNonNULBytes(t *testing.T) {
	// Success
	for _, c := range []struct {
		in       string
		expected []byte
	}{
		{"", []byte{}},
		{"\x01\x02\x03", []byte{1, 2, 3}},
		{"readable", []byte("readable")},
	} {
		out, err := stringToNonNULBytes(c.in)
		require.NoError(t, err, c.in)
		assert.Equal(t, c.expected, out, c.in)
	}

	// NUL in input is refused.
	for _, c := range []string{"\x00", "\x00X", "X\x00", "X\x00X"} {
		_, err := stringToNonNULBytes(c)
		assert.Error(t, err, c)
	}
}

// referenceNamed returns a reference.Named from input, or fails the test
func referenceNamed(t *testing.T, input string) reference.Named {
	ref, err := reference.ParseNormalizedNamed(input)
	require.NoError(t, err)
	return ref
}

// digestReference returns a reference.Reference from input, or fails the test
func digestReference(t *testing.T, input string) reference.Reference {
	ref, err := reference.ParseAnyReference(input)
	require.NoError(t, err)
	return ref
}

// fxDataBucketPath is the sequence of buckets needed to access fxDataKey in the underlying database.
var fxDataBucketPath = [][]byte{dataBucket, fxDataKey}

// createDBWithFxDataBucketPathPrefix returns a path to a temporary file which contains only
// pathPrefixLen elements of fxDataBucketPath.
func createDBWithFxDataBucketPathPrefix(t *testing.T, pathPrefixLen int) string {
	tmpFile, err := ioutil.TempFile("", "signatures-test-data-bucket-prefix")
	require.NoError(t, err)

	tx, commit := rwTx(t, tmpFile.Name())
	defer commit()
	if pathPrefixLen != 0 {
		b, err := tx.CreateBucketIfNotExists(fxDataBucketPath[0])
		require.NoError(t, err)
		for i := 1; i < pathPrefixLen; i++ {
			b, err = b.CreateBucketIfNotExists(fxDataBucketPath[i])
			require.NoError(t, err)
		}
	}

	return tmpFile.Name()
}

// testWithMissingFxDataSubBuckets runs fn with a path to a database in which
// no data exists, and some of the buckets on fxDataBucketPath may not exists either.
func testWithMissingFxDataSubBuckets(t *testing.T, fn func(path string)) {
	for prefixLen := 0; prefixLen < len(fxDataBucketPath); prefixLen++ {
		tmpPath := createDBWithFxDataBucketPathPrefix(t, prefixLen)
		defer os.Remove(tmpPath)
		fn(tmpPath)
	}
}

// testWithEmptyFxDataBucket runs fn with a path to a database where fxDataKey is empty
func testWithEmptyFxDataBucket(t *testing.T, fn func(path string)) {
	tmpPath := createDBWithFxDataBucketPathPrefix(t, len(fxDataBucketPath))
	defer os.Remove(tmpPath)
	fn(tmpPath)
}

// assertReadReturnedNothing verifies that (m, sigs, err) returned by Store.Read or readFromTx
// are empty with no error.
func assertReadReturnedNothing(t *testing.T, m []byte, sigs [][]byte, err error) {
	require.NoError(t, err)
	assert.Nil(t, m)
	assert.Empty(t, sigs)
}

// assertDBUnlocked verifies that a Bolt DB for s is not locked.
func assertDBUnlocked(t *testing.T, s *Store) {
	// Opening the database for writing attempts to acquire an exclusive lock; that guarantees
	// that the DB is locked neither for reading nor for writing.
	db, err := bolt.Open(s.signatureDBPath, 0600, &bolt.Options{Timeout: 1 * time.Millisecond})
	require.NoError(t, err)
	err = db.Close()
	require.NoError(t, err)
}

func TestStoreRead(t *testing.T) {
	s := NewStore(&types.SystemContext{DockerSignatureDBPath: fxDBPath})
	digestRef := digestReference(t, fxManifestDigest)

	// Success smoke test.  Most cases are tested in TestReadFromTx.
	namedRef := reference.TagNameOnly(referenceNamed(t, "busybox")) // Note that an untagged reference is ignored!
	m, sigs, err := s.Read(fxConfigDigest, namedRef)
	require.NoError(t, err)
	assert.Equal(t, fxManifestContents, m)
	assert.Equal(t, fxSignatures, sigs)
	assertDBUnlocked(t, s)

	// Error while reading/processing the data.
	_, _, err = s.Read(digest.Digest("X\x00X"), digestRef) // Invalid config digest
	assert.Error(t, err)
	assertDBUnlocked(t, s)

	// Database does not exist at all
	s = NewStore(&types.SystemContext{DockerSignatureDBPath: "/this/does/not/exist"})
	m, sigs, err = s.Read(fxConfigDigest, digestRef)
	assertReadReturnedNothing(t, m, sigs, err)

	// Database is unreadable
	tmpFile, err := ioutil.TempFile("", "signatures-newReader")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	err = tmpFile.Chmod(000)
	require.NoError(t, err)
	s = NewStore(&types.SystemContext{DockerSignatureDBPath: tmpFile.Name()})
	m, sigs, err = s.Read(fxConfigDigest, digestRef)
	assert.Error(t, err)

	// Failures closing the database are untested.
}

func TestReadFromTx(t *testing.T) {
	tx, cleanup := readTx(t, fxDBPath)
	defer cleanup()

	// Success.  Most cases of the naming handling are tested in TestResolveManifestDigest.
	named := reference.TagNameOnly(referenceNamed(t, "busybox")) // Note that an untagged reference is ignored!
	m, sigs, err := readFromTx(tx, fxConfigDigest, named)
	require.NoError(t, err)
	assert.Equal(t, fxManifestContents, m)
	assert.Equal(t, fxSignatures, sigs)
	// Manifest digest not found
	ref := referenceNamed(t, fxNamedTaggedReference.String()+"thisdoesnotexist")
	m, sigs, err = readFromTx(tx, fxConfigDigest, ref)
	assertReadReturnedNothing(t, m, sigs, err)
	// Error while resolving the manifest digest
	_, _, err = readFromTx(tx, digest.Digest("X\x00X"), named) // Invalid config digest
	assert.Error(t, err)

	digestRef := digestReference(t, fxManifestDigest)
	// One of the parent buckets in dataBucket does not exist
	readingPathReturnedNothing := func(path string) {
		tx, cleanup := readTx(t, path)
		defer cleanup()
		m, sigs, err := readFromTx(tx, fxConfigDigest, digestRef)
		assertReadReturnedNothing(t, m, sigs, err)
	}
	testWithMissingFxDataSubBuckets(t, readingPathReturnedNothing)
	// There is no data in the bucket
	testWithEmptyFxDataBucket(t, readingPathReturnedNothing)
	// Corrupt signatures
	testWithCorruptSignatures(t, func(description, path string) {
		tx, cleanup := readTx(t, path)
		defer cleanup()
		_, _, err := readFromTx(tx, fxConfigDigest, digestRef)
		assert.Error(t, err, description)
	})

	// Failures reading from the database are untested.
}

func TestStoreWrite(t *testing.T) {
	tmpFile, err := ioutil.TempFile("", "signatures-test-store.Write")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	// Success smoke test
	s := NewStore(&types.SystemContext{DockerSignatureDBPath: tmpFile.Name()})
	err = s.Write(fxConfigDigest, fxNamedTaggedReference, fxManifestContents, fxSignatures)
	assert.NoError(t, err)
	assertDBUnlocked(t, s)
	assertBoltDBMatchesFxDBDump(t, tmpFile.Name())

	// Error creating the database
	s = NewStore(&types.SystemContext{DockerSignatureDBPath: "/this/does/not/exist"})
	err = s.Write(fxConfigDigest, nil, fxManifestContents, [][]byte{})
	assert.Error(t, err)

	// Error opening the file for writing
	emptyDB := emptyBoltDB(t)
	defer os.Remove(emptyDB)
	err = os.Chmod(emptyDB, 000)
	require.NoError(t, err)
	s = NewStore(&types.SystemContext{DockerSignatureDBPath: emptyDB})
	err = s.Write(fxConfigDigest, nil, fxManifestContents, [][]byte{})
	assert.Error(t, err)

	// Failure closing the database is untested.
}

func TestWriteToTx(t *testing.T) {
	tmpFile, err := ioutil.TempFile("", "signatures-test-store.Write")
	require.NoError(t, err)

	// Success
	// Try things twice to see that reusing an existing database works and does not store two copies.
	for i := 0; i < 2; i++ {
		func() { // A scope for defer
			tx, commit := rwTx(t, tmpFile.Name())
			defer commit()
			err = writeToTx(tx, fxConfigDigest, fxNamedTaggedReference, fxManifestContents, fxSignatures)
			assert.NoError(t, err)
		}()
		assertBoltDBMatchesFxDBDump(t, tmpFile.Name())
	}

	func() { // A scope for defer
		tx, commit := rwTx(t, tmpFile.Name())
		defer commit()
		// Error computing digest
		err = writeToTx(tx, fxConfigDigest, nil, []byte(`{"schemaVersion":1,"signatures":1}`), [][]byte{})
		assert.Error(t, err)

		// Invalid config digest
		err = writeToTx(tx, digest.Digest("X\x00X"), nil, fxManifestContents, [][]byte{})
		assert.Error(t, err)
	}()

	// Error creating sub-buckets
	testWithMissingFxDataSubBuckets(t, func(path string) {
		tx, cleanup := readTx(t, path) // read-only, not rwTx
		defer cleanup()
		err := writeToTx(tx, fxConfigDigest, nil, fxManifestContents, fxSignatures)
		assert.Error(t, err)
	})

	// Failure updating signatures
	testWithCorruptSignatures(t, func(description, path string) {
		tx, commit := rwTx(t, path)
		defer commit()
		err := writeToTx(tx, fxConfigDigest, nil, fxManifestContents, fxSignatures)
		assert.Error(t, err)
	})

	// Failures creating dataKey and updating the manifest are untested:
	// Writing to a read-only transaction fails already on the first CreateBucketIfNotExists,
	// which detects a read-only transaction and fails. So, we can’t use a read-only transaction
	// to test write failures.
}

// fixtureReference emulates the reference recorded for the image in testdata/signatures.db
type fixtureReference struct{ ref reference.Named }

func (fr fixtureReference) Transport() types.ImageTransport {
	panic("not implemented")
}
func (fr fixtureReference) StringWithinTransport() string {
	panic("not implemented")
}
func (fr fixtureReference) DockerReference() reference.Named {
	return fr.ref
}
func (fr fixtureReference) PolicyConfigurationIdentity() string {
	panic("not implemented")
}
func (fr fixtureReference) PolicyConfigurationNamespaces() []string {
	panic("not implemented")
}
func (fr fixtureReference) NewImage(context.Context, *types.SystemContext) (types.ImageCloser, error) {
	panic("not implemented")
}
func (fr fixtureReference) NewImageSource(context.Context, *types.SystemContext) (types.ImageSource, error) {
	panic("not implemented")
}
func (fr fixtureReference) NewImageDestination(context.Context, *types.SystemContext) (types.ImageDestination, error) {
	panic("not implemented")
}
func (fr fixtureReference) DeleteImage(context.Context, *types.SystemContext) error {
	panic("not implemented")
}

// fixtureImageSource emulates the image recorded in testdata/signatures.db.
type fixtureImageSource struct{ ref reference.Named }

func (fis fixtureImageSource) Reference() types.ImageReference {
	return fixtureReference{ref: fis.ref}
}
func (fis fixtureImageSource) Close() error {
	return nil
}
func (fis fixtureImageSource) GetManifest(_ context.Context, instanceDigest *digest.Digest) ([]byte, string, error) {
	if instanceDigest != nil {
		panic("not implemented")
	}
	return fxManifestContents, manifest.DockerV2Schema2MediaType, nil
}
func (fis fixtureImageSource) GetOriginalManifest(context.Context, *digest.Digest) ([]byte, string, error) {
	panic("not implemented")
}
func (fis fixtureImageSource) HasThreadSafeGetBlob() bool {
	panic("not implemented")
}
func (fis fixtureImageSource) GetBlob(context.Context, types.BlobInfo, types.BlobInfoCache) (io.ReadCloser, int64, error) {
	panic("not implemented")
}
func (fis fixtureImageSource) GetSignatures(_ context.Context, instanceDigest *digest.Digest) ([][]byte, error) {
	if instanceDigest != nil {
		panic("not implemented")
	}
	return fxSignatures, nil
}
func (fis fixtureImageSource) LayerInfosForCopy(context.Context) ([]types.BlobInfo, error) {
	panic("not implemented")
}

func TestStoreRecordImage(t *testing.T) {
	tmpFile, err := ioutil.TempFile("", "signatures-test-store.RecordImage")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	s := NewStore(&types.SystemContext{DockerSignatureDBPath: tmpFile.Name()})

	fis := fixtureImageSource{ref: referenceNamed(t, fxNamedTaggedReference.String())}
	img, err := image.FromSource(context.Background(), nil, fis)
	require.NoError(t, err)
	err = s.RecordImage(context.Background(), img)
	assert.NoError(t, err)
	assertBoltDBMatchesFxDBDump(t, tmpFile.Name())
}

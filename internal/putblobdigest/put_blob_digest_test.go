package putblobdigest

import (
	"bytes"
	"io"
	"testing"

	"github.com/containers/image/v5/types"
	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testData = []byte("test data")

type testCase struct {
	inputDigest    digest.Digest
	computesDigest bool
	expectedDigest digest.Digest
}

func testDigester(t *testing.T, constructor func(io.Reader, types.BlobInfo) (Digester, io.Reader),
	cases []testCase) {
	for _, c := range cases {
		stream := bytes.NewReader(testData)
		digester, newStream := constructor(stream, types.BlobInfo{Digest: c.inputDigest})
		assert.Equal(t, c.computesDigest, newStream != stream, c.inputDigest)
		data, err := io.ReadAll(newStream)
		require.NoError(t, err, c.inputDigest)
		assert.Equal(t, testData, data, c.inputDigest)
		digest := digester.Digest()
		assert.Equal(t, c.expectedDigest, digest, c.inputDigest)
	}
}

func TestDigestIfUnknown(t *testing.T) {
	testDigester(t, DigestIfUnknown, []testCase{
		{
			inputDigest:    digest.Digest("sha256:uninspected-value"),
			computesDigest: false,
			expectedDigest: digest.Digest("sha256:uninspected-value"),
		},
		{
			inputDigest:    digest.Digest("unknown-algorithm:uninspected-value"),
			computesDigest: false,
			expectedDigest: digest.Digest("unknown-algorithm:uninspected-value"),
		},
		{
			inputDigest:    "",
			computesDigest: true,
			expectedDigest: digest.Canonical.FromBytes(testData),
		},
	})
}

func TestDigestIfCanonicalUnknown(t *testing.T) {
	testDigester(t, DigestIfCanonicalUnknown, []testCase{
		{
			inputDigest:    digest.Digest("sha256:uninspected-value"),
			computesDigest: false,
			expectedDigest: digest.Digest("sha256:uninspected-value"),
		},
		{
			inputDigest:    digest.Digest("unknown-algorithm:uninspected-value"),
			computesDigest: true,
			expectedDigest: digest.Canonical.FromBytes(testData),
		},
		{
			inputDigest:    "",
			computesDigest: true,
			expectedDigest: digest.Canonical.FromBytes(testData),
		},
	})
}

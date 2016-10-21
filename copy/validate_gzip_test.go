package copy

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Verify that the GZIP writer results are consistent across golang versions
//
// * a large payload is used to ensure the compression code utilizes several blocks
// * a small payload forces flushes in the compression code by using a partial block
//
// Verified against: go 1.7.3, go 1.6.3
func TestGzipWriter(t *testing.T) {
	cases := []struct {
		name   string
		digest string
	}{
		{"fixtures/random_512k.bin", "89ec079aa14317e6a96c53dc22081ec054c47533cea5858cc18644bfb148a363"},
		{"fixtures/random_500b.bin", "0b3a69e34b8ea6d544b1686aaabdcb162a4e3ea597a320a32107b6eb74c125e1"},
	}

	for _, test := range cases {
		payload, err := ioutil.ReadFile(test.name)
		require.NoError(t, err, "failed to read payload from disk")

		var buffer bytes.Buffer
		zipper := gzip.NewWriter(&buffer)
		_, err = zipper.Write(payload)
		require.NoError(t, err, "failed to write payload to gzip.Writer")

		// Flush and finialize all data being zipped
		err = zipper.Close()
		require.NoError(t, err, "failed to close gzip.Writer")

		var actual bytes.Buffer
		_, err = buffer.WriteTo(&actual)
		require.NoError(t, err, "failed to retrieve payload")

		sum := sha256.Sum256(actual.Bytes())
		digest := hex.EncodeToString(sum[:])

		// If checksum does not match then something has changed in the gzip library
		assert.Equal(t, test.digest, digest, fmt.Sprintf("Invalid payload sha256: %v", test.name))
	}
}

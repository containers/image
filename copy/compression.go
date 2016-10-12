package copy

import (
	"bytes"
	"io"

	"github.com/Sirupsen/logrus"
)

// compressionPrefixes is an internal implementation detail of isStreamCompressed
var compressionPrefixes = map[string][]byte{
	"gzip":  {0x1F, 0x8B, 0x08},                   // gzip (RFC 1952)
	"bzip2": {0x42, 0x5A, 0x68},                   // bzip2 (decompress.c:BZ2_decompress)
	"xz":    {0xFD, 0x37, 0x7A, 0x58, 0x5A, 0x00}, // xz (/usr/share/doc/xz/xz-file-format.txt)
}

// isStreamCompressed returns true if input is recognized as a compressed format.
// Because it consumes the start of input, other consumers must use the returned io.Reader instead to also read from the beginning.
func isStreamCompressed(input io.Reader) (bool, io.Reader, error) {
	buffer := [8]byte{}

	n, err := io.ReadAtLeast(input, buffer[:], len(buffer))
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		// This is a “real” error. We could just ignore it this time, process the data we have, and hope that the source will report the same error again.
		// Instead, fail immediately with the original error cause instead of a possibly secondary/misleading error returned later.
		return false, nil, err
	}

	isCompressed := false
	for algo, prefix := range compressionPrefixes {
		if bytes.HasPrefix(buffer[:n], prefix) {
			logrus.Debugf("Detected compression format %s", algo)
			isCompressed = true
			break
		}
	}
	if !isCompressed {
		logrus.Debugf("No compression detected")
	}

	return isCompressed, io.MultiReader(bytes.NewReader(buffer[:n]), input), nil
}

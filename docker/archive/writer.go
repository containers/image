package archive

import (
	"os"

	"github.com/pkg/errors"
)

// openArchiveForWriting opens path for writing a tar archive,
// making a few sanity checks.
func openArchiveForWriting(path string) (*os.File, error) {
	// path can be either a pipe or a regular file
	// in the case of a pipe, we require that we can open it for write
	// in the case of a regular file, we don't want to overwrite any pre-existing file
	// so we check for Size() == 0 below (This is racy, but using O_EXCL would also be racy,
	// only in a different way. Either way, it’s up to the user to not have two writers to the same path.)
	fh, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return nil, errors.Wrapf(err, "error opening file %q", path)
	}
	succeeded := false
	defer func() {
		if !succeeded {
			fh.Close()
		}
	}()
	fhStat, err := fh.Stat()
	if err != nil {
		return nil, errors.Wrapf(err, "error statting file %q", path)
	}

	if fhStat.Mode().IsRegular() && fhStat.Size() != 0 {
		return nil, errors.New("docker-archive doesn't support modifying existing images")
	}

	succeeded = true
	return fh, nil
}

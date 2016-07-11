package directory

import (
	"io"
	"io/ioutil"
	"os"

	"github.com/containers/image/types"
	"github.com/docker/docker/reference"
)

type dirImageDestination struct {
	dir string
}

// NewImageDestination returns an ImageDestination for writing to an existing directory.
func NewImageDestination(dir string) types.ImageDestination {
	return &dirImageDestination{dir}
}

func (d *dirImageDestination) CanonicalDockerReference() reference.Named {
	return nil
}

func (d *dirImageDestination) SupportedManifestMIMETypes() []string {
	return nil
}

func (d *dirImageDestination) PutManifest(manifest []byte) error {
	return ioutil.WriteFile(manifestPath(d.dir), manifest, 0644)
}

func (d *dirImageDestination) PutBlob(digest string, stream io.Reader) error {
	layerFile, err := os.Create(layerPath(d.dir, digest))
	if err != nil {
		return err
	}
	defer layerFile.Close()
	if _, err := io.Copy(layerFile, stream); err != nil {
		return err
	}
	if err := layerFile.Sync(); err != nil {
		return err
	}
	return nil
}

func (d *dirImageDestination) PutSignatures(signatures [][]byte) error {
	for i, sig := range signatures {
		if err := ioutil.WriteFile(signaturePath(d.dir, i), sig, 0644); err != nil {
			return err
		}
	}
	return nil
}

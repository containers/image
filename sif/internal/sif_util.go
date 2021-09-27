package internal

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/opencontainers/go-digest"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
	"github.com/yhcote/sif/pkg/sif"

	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

type SifImage struct {
	fimg      sif.FileImage
	rootfs    *sif.Descriptor
	deffile   *sif.Descriptor
	defReader *io.SectionReader
	cmdlist   []string
	runscript *bytes.Buffer
	env       *sif.Descriptor
	envReader *io.SectionReader
	envlist   []string
	diffID    digest.Digest
}

func LoadSIFImage(path string) (image SifImage, err error) {
	// open up the SIF file and get its header
	image.fimg, err = sif.LoadContainer(path, true)
	if err != nil {
		return
	}

	// check for a system partition and save it
	image.rootfs, _, err = image.fimg.GetPartPrimSys()
	if err != nil {
		return SifImage{}, errors.Wrap(err, "looking up rootfs from SIF file")
	}

	// look for a definition file object
	searchDesc := sif.Descriptor{Datatype: sif.DataDeffile}
	resultDescs, _, err := image.fimg.GetFromDescr(searchDesc)
	if err == nil && resultDescs != nil {
		// we assume in practice that typical SIF files don't hold multiple deffiles
		image.deffile = resultDescs[0]
		image.defReader = io.NewSectionReader(image.fimg.Fp, image.deffile.Fileoff, image.deffile.Filelen)
	}
	if err = image.generateConfig(); err != nil {
		return SifImage{}, err
	}

	// look for an environment variable set object
	searchDesc = sif.Descriptor{Datatype: sif.DataEnvVar}
	resultDescs, _, err = image.fimg.GetFromDescr(searchDesc)
	if err == nil && resultDescs != nil {
		// we assume in practice that typical SIF files don't hold multiple EnvVar sets
		image.env = resultDescs[0]
		image.envReader = io.NewSectionReader(image.fimg.Fp, image.env.Fileoff, image.env.Filelen)
	}

	return image, nil
}

func (image *SifImage) parseEnvironment(scanner *bufio.Scanner) error {
	for scanner.Scan() {
		s := strings.TrimSpace(scanner.Text())
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		if strings.HasPrefix(s, "%") {
			return nil
		}
		image.envlist = append(image.envlist, s)
	}
	if err := scanner.Err(); err != nil {
		return errors.Wrap(err, "parsing environment from SIF definition file object")
	}
	return nil
}

func (image *SifImage) parseRunscript(scanner *bufio.Scanner) error {
	for scanner.Scan() {
		s := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(s, "%") {
			return nil
		}
		image.cmdlist = append(image.cmdlist, s)
	}
	if err := scanner.Err(); err != nil {
		return errors.Wrap(err, "parsing runscript from SIF definition file object")
	}
	return nil
}

func (image *SifImage) generateRunscript() error {
	base := `#!/bin/bash
`
	image.runscript = bytes.NewBufferString(base)
	for _, s := range image.envlist {
		_, err := image.runscript.WriteString(fmt.Sprintln(s))
		if err != nil {
			return errors.Wrap(err, "writing to runscript buffer")
		}
	}
	for _, s := range image.cmdlist {
		_, err := image.runscript.WriteString(fmt.Sprintln(s))
		if err != nil {
			return errors.Wrap(err, "writing to runscript buffer")
		}
	}
	return nil
}

func (image *SifImage) generateConfig() error {
	if image.deffile == nil {
		image.cmdlist = append(image.cmdlist, "bash")
		return nil
	}

	// extract %environment/%runscript from definition file
	var err error
	scanner := bufio.NewScanner(image.defReader)
	for scanner.Scan() {
		s := strings.TrimSpace(scanner.Text())
	again:
		if s == `%environment` {
			if err = image.parseEnvironment(scanner); err != nil {
				return err
			}
		} else if s == `%runscript` {
			if err = image.parseRunscript(scanner); err != nil {
				return err
			}
		}
		s = strings.TrimSpace(scanner.Text())
		if s == `%environment` || s == `%runscript` {
			goto again
		}
	}
	if err := scanner.Err(); err != nil {
		return errors.Wrap(err, "reading lines from SIF definition file object")
	}

	if len(image.cmdlist) == 0 && len(image.envlist) == 0 {
		image.cmdlist = append(image.cmdlist, "bash")
	} else {
		image.generateRunscript()
		image.cmdlist = []string{"/podman/runscript"}
	}

	return nil
}

func (image SifImage) GetConfig(config *imgspecv1.Image) error {
	config.Config.Cmd = append(config.Config.Cmd, image.cmdlist...)
	return nil
}

func (image SifImage) UnloadSIFImage() (err error) {
	err = image.fimg.UnloadContainer()
	return
}

func (image SifImage) GetSIFID() string {
	return image.fimg.Header.ID.String()
}

func (image SifImage) GetSIFArch() string {
	return sif.GetGoArch(string(image.fimg.Header.Arch[:sif.HdrArchLen-1]))
}

const squashFilename = "rootfs.squashfs"
const tarFilename = "rootfs.tar"

func runUnSquashFSTar(tempdir string) (err error) {
	script := `
#!/bin/sh
unsquashfs -f ` + squashFilename + ` && tar --acls --xattrs -C ./squashfs-root -cpf ` + tarFilename + ` ./
`

	if err = ioutil.WriteFile(filepath.Join(tempdir, "script"), []byte(script), 0755); err != nil {
		return err
	}
	cmd := []string{"fakeroot", "--", "./script"}

	xcmd := exec.Command(cmd[0], cmd[1:]...)
	xcmd.Stderr = os.Stderr
	xcmd.Dir = tempdir
	err = xcmd.Run()
	return
}

func (image *SifImage) writeRunscript(tempdir string) (err error) {
	if image.runscript == nil {
		return nil
	}
	rsPath := filepath.Join(tempdir, "squashfs-root", "podman")
	if err = os.MkdirAll(rsPath, 0755); err != nil {
		return
	}
	if err = ioutil.WriteFile(filepath.Join(rsPath, "runscript"), image.runscript.Bytes(), 0755); err != nil {
		return errors.Wrap(err, "writing /podman/runscript")
	}
	return nil
}

func (image SifImage) SquashFSToTarLayer(tempdir string) (tarpath string, err error) {
	if _, err = image.fimg.Fp.Seek(image.rootfs.Fileoff, 0); err != nil {
		return
	}
	f, err := os.Create(filepath.Join(tempdir, squashFilename))
	if err != nil {
		return
	}
	defer f.Close()
	if _, err = io.CopyN(f, image.fimg.Fp, image.rootfs.Filelen); err != nil {
		return
	}
	if err = f.Sync(); err != nil {
		return
	}
	if err = image.writeRunscript(tempdir); err != nil {
		return
	}
	if err = runUnSquashFSTar(tempdir); err != nil {
		return
	}
	return filepath.Join(tempdir, tarFilename), nil
}

func createSIF() error {
	cinfo := sif.CreateInfo{
		Pathname:   "container.sif",
		Launchstr:  sif.HdrLaunch,
		Sifversion: sif.HdrVersion,
		ID:         uuid.NewV4(),
	}

	image, err := sif.CreateContainer(cinfo)
	if err != nil {
		return err
	}
	fmt.Printf("SIF: %+v\n", image)

	return nil
}

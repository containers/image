//go:build linux
// +build linux

package sifimage

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

	"github.com/pkg/errors"
	"github.com/sylabs/sif/v2/pkg/sif"

	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

type loadedSifImage struct {
	fimg      *sif.FileImage
	rootfs    sif.Descriptor
	deffile   *sif.Descriptor
	defReader io.Reader
	cmdlist   []string
	runscript *bytes.Buffer
	env       *sif.Descriptor
	envReader io.Reader
	envlist   []string
}

func loadSIFImage(path string) (image loadedSifImage, err error) {
	// open up the SIF file and get its header
	image.fimg, err = sif.LoadContainerFromPath(path, sif.OptLoadWithFlag(os.O_RDONLY))
	if err != nil {
		return
	}

	// check for a system partition and save it
	image.rootfs, err = image.fimg.GetDescriptor(sif.WithPartitionType(sif.PartPrimSys))
	if err != nil {
		return loadedSifImage{}, errors.Wrap(err, "looking up rootfs from SIF file")
	}

	// look for a definition file object
	resultDesc, err := image.fimg.GetDescriptor(sif.WithDataType(sif.DataDeffile))
	if err == nil {
		// we assume in practice that typical SIF files don't hold multiple deffiles
		image.deffile = &resultDesc
		image.defReader = resultDesc.GetReader()
	}
	if err = image.generateConfig(); err != nil {
		return loadedSifImage{}, err
	}

	// look for an environment variable set object
	resultDesc, err = image.fimg.GetDescriptor(sif.WithDataType(sif.DataEnvVar))
	if err == nil {
		// we assume in practice that typical SIF files don't hold multiple EnvVar sets
		image.env = &resultDesc
		image.envReader = resultDesc.GetReader()
	}

	return image, nil
}

func (image *loadedSifImage) parseEnvironment(scanner *bufio.Scanner) error {
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

func (image *loadedSifImage) parseRunscript(scanner *bufio.Scanner) error {
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

func (image *loadedSifImage) generateRunscript() error {
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

func (image *loadedSifImage) generateConfig() error {
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
		if err = image.generateRunscript(); err != nil {
			return errors.Wrap(err, "generating runscript")
		}
		image.cmdlist = []string{"/podman/runscript"}
	}

	return nil
}

func (image loadedSifImage) GetConfig(config *imgspecv1.Image) error {
	config.Config.Cmd = append(config.Config.Cmd, image.cmdlist...)
	return nil
}

func (image loadedSifImage) UnloadSIFImage() (err error) {
	err = image.fimg.UnloadContainer()
	return
}

func (image loadedSifImage) GetSIFID() string {
	return image.fimg.ID()
}

func (image loadedSifImage) GetSIFArch() string {
	return image.fimg.PrimaryArch()
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

func (image *loadedSifImage) writeRunscript(tempdir string) (err error) {
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

func (image loadedSifImage) SquashFSToTarLayer(tempdir string) (tarpath string, err error) {
	f, err := os.Create(filepath.Join(tempdir, squashFilename))
	if err != nil {
		return
	}
	defer f.Close()
	if _, err = io.CopyN(f, image.rootfs.GetReader(), image.rootfs.Size()); err != nil {
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

package main

import (
	"errors"
	"fmt"
	"io/ioutil"

	"github.com/projectatomic/skopeo/signature"
	"github.com/urfave/cli"
)

func standaloneSign(context *cli.Context) error {
	outputFile := context.String("output")
	if len(context.Args()) != 3 || outputFile == "" {
		return errors.New("Usage: skopeo standalone-sign manifest docker-reference key-fingerprint -o signature")
	}
	manifestPath := context.Args()[0]
	dockerReference := context.Args()[1]
	fingerprint := context.Args()[2]

	manifest, err := ioutil.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("Error reading %s: %v", manifestPath, err)
	}

	mech, err := signature.NewGPGSigningMechanism()
	if err != nil {
		return fmt.Errorf("Error initializing GPG: %v", err)
	}
	signature, err := signature.SignDockerManifest(manifest, dockerReference, mech, fingerprint)
	if err != nil {
		return fmt.Errorf("Error creating signature: %v", err)
	}

	if err := ioutil.WriteFile(outputFile, signature, 0644); err != nil {
		return fmt.Errorf("Error writing signature to %s: %v", outputFile, err)
	}
	return nil
}

var standaloneSignCmd = cli.Command{
	Name:   "standalone-sign",
	Usage:  "Create a signature using local files",
	Action: standaloneSign,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "output, o",
			Usage: "output signature file name",
		},
	},
}

func standaloneVerify(context *cli.Context) error {
	if len(context.Args()) != 4 {
		return errors.New("Usage: skopeo standalone-verify manifest docker-reference key-fingerprint signature")
	}
	manifestPath := context.Args()[0]
	expectedDockerReference := context.Args()[1]
	expectedFingerprint := context.Args()[2]
	signaturePath := context.Args()[3]

	unverifiedManifest, err := ioutil.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("Error reading manifest from %s: %v", signaturePath, err)
	}
	unverifiedSignature, err := ioutil.ReadFile(signaturePath)
	if err != nil {
		return fmt.Errorf("Error reading signature from %s: %v", signaturePath, err)
	}

	mech, err := signature.NewGPGSigningMechanism()
	if err != nil {

		return fmt.Errorf("Error initializing GPG: %v", err)
	}
	sig, err := signature.VerifyDockerManifestSignature(unverifiedSignature, unverifiedManifest, expectedDockerReference, mech, expectedFingerprint)
	if err != nil {
		return fmt.Errorf("Error verifying signature: %v", err)
	}

	fmt.Printf("Signature verified, digest %s\n", sig.DockerManifestDigest)
	return nil
}

var standaloneVerifyCmd = cli.Command{
	Name:   "standalone-verify",
	Usage:  "Verify a signature using local files",
	Action: standaloneVerify,
}

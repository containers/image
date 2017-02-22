package main

import (
	"os"

	"github.com/containers/image/copy"
	"github.com/containers/image/docker/daemon"
	"github.com/containers/image/signature"
	"github.com/containers/image/types"
	"github.com/docker/distribution/reference"
)

func main() {
	ref, err := daemon.ParseReference("docker.io/library/golang:latest")
	if err != nil {
		panic(err)
	}

	tgt, err := reference.ParseNamed("docker.io/erikh/test:latest")
	if err != nil {
		panic(err)
	}

	tgtRef, err := daemon.NewReference("", tgt)
	if err != nil {
		panic(err)
	}

	pc, err := signature.NewPolicyContext(&signature.Policy{
		Default: []signature.PolicyRequirement{signature.NewPRInsecureAcceptAnything()},
	})

	if err != nil {
		panic(err)
	}

	err = copy.Image(pc, tgtRef, ref, &copy.Options{
		LayerEditor: func(input []types.BlobInfo) []types.BlobInfo {
			return input[:2]
		},
		RemoveSignatures: true,
		ReportWriter:     os.Stdout,
	})
	if err != nil {
		panic(err)
	}
}

package main

import (
	"encoding/json"
	"fmt"

	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
)

// FIXME: Also handle schema2, and put this elsewhere:
// docker.go contains similar code, more sophisticated
// (at the very least the deduplication should be reused from there).
func manifestLayers(manifest []byte) ([]string, error) {
	man := manifestSchema1{}
	if err := json.Unmarshal(manifest, &man); err != nil {
		return nil, err
	}

	layers := []string{}
	for _, layer := range man.FSLayers {
		layers = append(layers, layer.BlobSum)
	}
	return layers, nil
}

func copyHandler(context *cli.Context) {
	if len(context.Args()) != 2 {
		logrus.Fatal("Usage: copy source destination")
	}

	src, err := parseImageSource(context, context.Args()[0])
	if err != nil {
		logrus.Fatalf("Error initializing %s: %s", context.Args()[0], err.Error())
	}

	dest, err := parseImageDestination(context, context.Args()[1])
	if err != nil {
		logrus.Fatalf("Error initializing %s: %s", context.Args()[1], err.Error())
	}

	manifest, digest, err := src.GetManifest()
	if err != nil {
		logrus.Fatalf("Error reading manifest: %s", err.Error())
	}
	fmt.Printf("Canonical manifest digest: %s\n", digest)

	layers, err := manifestLayers(manifest)
	if err != nil {
		logrus.Fatalf("Error parsing manifest: %s", err.Error())
	}
	for _, layer := range layers {
		stream, err := src.GetLayer(layer)
		if err != nil {
			logrus.Fatalf("Error reading layer %s: %s", layer, err.Error())
		}
		defer stream.Close()
		if err := dest.PutLayer(layer, stream); err != nil {
			logrus.Fatalf("Error writing layer: %s", err.Error())
		}
	}

	sigs, err := src.GetSignatures()
	if err != nil {
		logrus.Fatalf("Error reading signatures: %s", err.Error())
	}
	if err := dest.PutSignatures(sigs); err != nil {
		logrus.Fatalf("Error writing signatures: %s", err.Error())
	}

	// FIXME: We need to call PutManifest after PutLayer and PutSignatures. This seems ugly; move to a "set properties" + "commit" model?
	if err := dest.PutManifest(manifest); err != nil {
		logrus.Fatalf("Error writing manifest: %s", err.Error())
	}
}

var copyCmd = cli.Command{
	Name:   "copy",
	Action: copyHandler,
}

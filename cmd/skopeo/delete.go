package main

import (
	"errors"

	"github.com/urfave/cli"
)

func deleteHandler(context *cli.Context) error {
	if len(context.Args()) != 1 {
		return errors.New("Usage: delete imageReference")
	}

	image, err := parseImageSource(context, context.Args()[0])
	if err != nil {
		return err
	}

	if err := image.Delete(); err != nil {
		return err
	}
	return nil
}

var deleteCmd = cli.Command{
	Name:   "delete",
	Usage:  "Delete given image",
	Action: deleteHandler,
}

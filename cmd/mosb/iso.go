package main

import (
	"fmt"

	"github.com/project-machine/mos/pkg/mosconfig"
	"github.com/urfave/cli"
)

var isoCmd = cli.Command{
	Name:  "iso",
	Usage: "build/inspect mos install iso image",
	Subcommands: []cli.Command{
		cli.Command{
			Name:   "build",
			Action: doBuildISO,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "key",
					Usage: "path to manifest signing key to use",
					Value: "",
				},
				cli.StringFlag{
					Name:  "cert",
					Usage: "path to manifest certificate to use",
					Value: "",
				},
				cli.StringFlag{
					Name:  "file",
					Usage: "path to the file with targets list",
					Value: "targets.yaml",
				},
				cli.StringFlag{
					Name:  "output-file",
					Usage: "path to which to write the built ISO image",
					Value: "mos.iso",
				},
				cli.StringFlag{
					Name:  "update-type",
					Usage: "Update type, complete or partial",
					Value: "complete",
				},
			},
		},
	},
}

func doBuildISO(ctx *cli.Context) error {
	cert := ctx.String("cert")
	if cert == "" {
		return fmt.Errorf("Certificate filename is required")
	}

	key := ctx.String("key")
	if key == "" {
		return fmt.Errorf("Key filename is required")
	}

	updateType, err := mosconfig.ParseUpdateType(ctx.String("update-type"))
	if err != nil {
		return err
	}

	// TODO product should come from certificate
	product := "de6c82c5-2e01-4c92-949b-a6545d30fc06"

	iso := mosconfig.ISOConfig{
		InputFile:  ctx.String("file"),
		OutputFile: ctx.String("output-file"),
		Product:    product,
		Cert:       cert,
		Key:        key,
		UpdateType: updateType,
	}

	// TODO - do we need to do some cosign integration for
	// verifying containers, or can that be done automatically?
	return iso.Generate()
}

package main

import (
	"fmt"

	"github.com/project-machine/mos/pkg/mosconfig"
	"github.com/urfave/cli"
)

var sociCmd = cli.Command{
	Name:  "soci",
	Usage: "install a new mos system",
	Subcommands: []cli.Command{
		cli.Command{
			Name:   "build",
			Action: doBuildSOCI,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "oci-layer",
					Usage: "target OCI layer which meta layer should reference",
					Value: "",
				},
				cli.StringFlag{
					Name:  "zot-path",
					Usage: "Zot path on host",
					Value: "",
				},
				cli.StringFlag{
					Name:  "target-name",
					Usage: "name to assign this target",
					Value: "hostfs",
				},
				cli.StringFlag{
					Name:  "version",
					Usage: "version number to assign to this target",
					Value: "0.0.1",
				},
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
					Name:  "soci-layer",
					Usage: "OCI path for signed oci layer to create",
					Value: "oci:meta",
				},
			},
		},
	},
}

func doBuildSOCI(ctx *cli.Context) error {
	cert := ctx.String("cert")
	if cert == "" {
		return fmt.Errorf("Certificate filename is required")
	}

	key := ctx.String("key")
	if key == "" {
		return fmt.Errorf("Key filename is required")
	}

	zotpath := ctx.String("zot-path")
	if zotpath == "" {
		return fmt.Errorf("Zot path is required")
	}

	layer := ctx.String("oci-layer")
	targetname := ctx.String("target-name")
	meta := ctx.String("soci-layer")
	version := ctx.String("version")

	soci := mosconfig.SOCI{
		Layer:       layer,
		ZotPath:     zotpath,
		ServiceName: targetname,
		Version:     version,
		Meta:        meta,
		Cert:        cert,
		Key:         key,
	}

	// TODO - do we need to do some cosign integration for
	// verifying containers, or can that be done automatically?
	return soci.Generate()
}

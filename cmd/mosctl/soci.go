package main

import (
	"github.com/project-machine/mos/pkg/mosconfig"
	"github.com/urfave/cli"

	"github.com/pkg/errors"
)

var sociCmd = cli.Command{
	Name: "soci",
	Usage: "install a new mos system",
	Action: doInstall,
	Subcommands: []cli.Command{
		cli.Command{
			Name: "mount",
			Action: mountSOci,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name: "ocidir",
					Usage: "OCI directory",
					Value: "oci",
				},
				cli.StringFlag{
					Name: "meta, metalayer, zaplayer",
					Usage: "zap (oci metadata) layer name",
					Value: "meta",
				},
				cli.StringFlag{
					Name: "capath, ca",
					Usage: "Path to manifest signing CA",
					Value: "/factory/secure/manifestCA.pem",
				},
				cli.StringFlag{
					Name: "mountpoint, dest",
					Usage: "Directory onto which to mount the layer",
				},
			},
		},
	},
}

func mountSOci(ctx *cli.Context) error {
	mp := ctx.String("dest")
	if mp == "" {
		return errors.Errorf("mountpoint is mandatory")
	}
	if !mosconfig.PathExists(mp) {
		return errors.Errorf("mountpoint does not exist")
	}
	ocidir := ctx.String("ocidir")
	metalayer := ctx.String("meta")
	capath := ctx.String("ca")

	return mosconfig.MountSOCI(ocidir, metalayer, capath, mp)
}

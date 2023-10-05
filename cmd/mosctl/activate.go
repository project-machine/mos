package main

import (
	"fmt"

	"github.com/project-machine/mos/pkg/mosconfig"
	"github.com/project-machine/mos/pkg/utils"
	"github.com/urfave/cli"
)

var activateCmd = cli.Command{
	Name:   "activate",
	Usage:  "activate (start or re-start) a service",
	Action: doActivate,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "t, target",
			Usage: "Target to activate.  hostfs (the default) means boot/reboot",
			Value: "hostfs",
		},
		cli.StringFlag{
			Name:  "root, rfs, r",
			Usage: "Directory under which to find the mos install",
			Value: "/",
		},
		cli.StringFlag{
			Name:  "capath, ca",
			Usage: "Manifest CA path",
			Value: "/factory/secure/manifestCA.pem",
		},
	},
}

func doActivate(ctx *cli.Context) error {
	rfs := ctx.String("root")
	if rfs == "" || !utils.PathExists(rfs) {
		return fmt.Errorf("A valid root directory must be specified")
	}

	target := ctx.String("target")
	if target == "" {
		target = "hostfs"
	}

	opts := mosconfig.DefaultMosOptions()
	opts.RootDir = rfs
	capath := ctx.String("capath")
	if capath != "" {
		opts.CaPath = capath
	}
	mos, err := mosconfig.OpenMos(opts)
	if err != nil {
		return fmt.Errorf("Failed opening mos: %w", err)
	}

	err = mos.Activate(target)
	if err != nil {
		return fmt.Errorf("Failed to activate %s: %w", target, err)
	}

	return nil
}

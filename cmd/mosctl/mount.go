package main

import (
	"fmt"

	"github.com/apex/log"
	"github.com/project-machine/mos/pkg/mosconfig"
	"github.com/project-machine/mos/pkg/utils"
	"github.com/urfave/cli"
)

var mountCmd = cli.Command{
	Name:   "mount",
	Usage:  "mount a service filesystem, from host manifest or manifest at OCI URL (positional argument)",
	Action: doMount,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "target",
			Usage: "Target to activate.  hostfs (the default) means boot/reboot",
			Value: "hostfs",
		},
		cli.BoolFlag{
			Name:  "readonly, ro",
			Usage: "Mount as readonly",
		},
		cli.StringFlag{
			Name:  "dest",
			Usage: "Directory over which to mount",
		},
		cli.StringFlag{ // TODO - we will want to drop the rfs option eventually...
			Name:  "root, rfs, r",
			Usage: "Directory under which to find the mos install",
			Value: "/",
		},
	},
}

func doMount(ctx *cli.Context) error {
	rfs := ctx.String("root")
	if rfs == "" || !utils.PathExists(rfs) {
		return fmt.Errorf("A valid root directory must be specified")
	}

	url := ""
	if len(ctx.Args()) != 0 {
		url = ctx.Args()[0]
	}

	target := "hostfs"
	if ctx.IsSet("target") {
		target = ctx.String("target")
	} else if url != "" {
		target = "livecd"
	}

	dest := ""
	if ctx.IsSet("dest") {
		dest = ctx.String("dest")
	}

	opts := mosconfig.DefaultMosOptions()
	opts.RootDir = rfs

	mos, err := mosconfig.OpenMos(opts)
	if err != nil {
		return fmt.Errorf("Failed opening mos: %w", err)
	}

	err = mos.Mount(target, dest, url, ctx.Bool("readonly"))
	if err != nil {
		return fmt.Errorf("Failed to activate %s: %w", target, err)
	}

	log.Debugf("%s has been setup under %s", target, dest)
	return nil
}

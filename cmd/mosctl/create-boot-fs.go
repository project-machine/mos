package main

import (
	"github.com/apex/log"
	"github.com/pkg/errors"
	"github.com/project-machine/mos/pkg/mosconfig"
	"github.com/urfave/cli"
)

var createBootFsCmd = cli.Command{
	Name:   "create-boot-fs",
	Usage:  "Create a boot filesystem",
	Action: doCreateBootfs,
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "readonly,ro",
			Usage: "Make mount read-only",
		},
		cli.StringFlag{
			Name:  "rfs, root, r",
			Usage: "Directory under which to find the mos install",
			Value: "/",
		},
		cli.StringFlag{
			Name:  "dest",
			Usage: "Directory over which to mount the rfs",
			Value: "/sysroot",
		},
	},
}

// Setup a rootfs to which dracut should pivot.
// Note, setup of luks keys, SUDI keys, and extension of PCR7
// must already have been done.
func doCreateBootfs(ctx *cli.Context) error {
	opts := mosconfig.DefaultMosOptions()

	if ctx.IsSet("rfs") {
		opts.RootDir = ctx.String("rfs")
	}


	mos, err := mosconfig.OpenMos(opts)
	if err != nil {
		return errors.Wrapf(err, "Error opening mos")
	}

	dest := ctx.String("dest")
	err = mos.Mount("hostfs", dest, "", ctx.Bool("readonly"))
	if err != nil {
		return errors.Wrapf(err, "Error mounting rootfs %q", dest)
	}

	log.Debugf("Rootfs has been setup under %s", dest)
	return nil
}

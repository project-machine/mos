package main

import (
	"fmt"

	"github.com/project-machine/mos/pkg/mosconfig"
	"github.com/urfave/cli"
)

var updateCmd = cli.Command{
	Name:   "update",
	Usage:  "update a mos system",
	Action: doUpdate,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "f, file",
			Usage: "File from which to read the install manifest",
			Value: "./install.yaml",
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

func doUpdate(ctx *cli.Context) error {
	rfs := ctx.String("root")
	if rfs == "" || !mosconfig.PathExists(rfs) {
		return fmt.Errorf("A valid root directory must be specified")
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
	defer mos.Close()

	cpath := ctx.String("file")
	err = mos.Update(cpath)
	if err != nil {
		return fmt.Errorf("Update using %q failed: %w", cpath, err)
	}

	return nil
}

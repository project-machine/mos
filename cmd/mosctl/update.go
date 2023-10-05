package main

import (
	"fmt"
	"path/filepath"

	"github.com/project-machine/mos/pkg/mosconfig"
	"github.com/project-machine/mos/pkg/utils"
	"github.com/urfave/cli"
)

var updateCmd = cli.Command{
	Name:   "update",
	Usage:  "update a mos system",
	Action: doUpdate,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "root, rfs, r",
			Usage: "Directory under which to find the mos install",
			Value: "/",
		},
	},
}

func doUpdate(ctx *cli.Context) error {
	rfs := ctx.String("root")
	if rfs == "" || !utils.PathExists(rfs) {
		return fmt.Errorf("A valid root directory must be specified")
	}

	opts := mosconfig.DefaultMosOptions()
	opts.RootDir = rfs
	capath := filepath.Join(rfs, "factory/secure/manifestCA.pem")
	if ctx.IsSet(capath) {
		opts.CaPath = capath
	}

	mos, err := mosconfig.OpenMos(opts)
	if err != nil {
		return fmt.Errorf("Failed opening mos: %w", err)
	}
	defer mos.Close()

	if len(ctx.Args()) != 1 {
		return fmt.Errorf("update requires an oci url for update manifest")
	}
	url := ctx.Args()[0]
	err = mos.Update(url)
	if err != nil {
		return fmt.Errorf("Update using %q failed: %w", url, err)
	}

	return nil
}

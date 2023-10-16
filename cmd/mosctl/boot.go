package main

import (
	"github.com/pkg/errors"
	"github.com/project-machine/mos/pkg/mosconfig"
	"github.com/urfave/cli"
)

var bootCmd = cli.Command{
	Name:   "boot",
	Usage:  "start all services listed in mos manifest",
	Action: doBootCmd,
}

func doBootCmd(ctx *cli.Context) error {
	opts := mosconfig.DefaultMosOptions()
	mos, err := mosconfig.OpenMos(opts)
	if err != nil {
		return errors.Wrapf(err, "Failed opening mos")
	}

	err = mos.Boot()
	if err != nil {
		return errors.Wrapf(err, "Failed to boot")
	}

	return nil
}

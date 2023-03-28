package main

import (
	"github.com/project-machine/mos/pkg/mosconfig"
	"github.com/urfave/cli"
)

var installCmd = cli.Command{
	Name:   "install",
	Usage:  "install a new mos system",
	Action: doInstall,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "root, rfs, r",
			Usage: "Directory under which to find the mos install",
			Value: "/",
		},
	},
}

func doInstall(ctx *cli.Context) error {
	if err := mosconfig.InitializeMos(ctx); err != nil {
		return err
	}

	return nil
}

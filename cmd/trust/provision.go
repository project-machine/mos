package main

import (
	"fmt"

	"github.com/apex/log"
	"github.com/project-machine/mos/pkg/trust"
	"github.com/urfave/cli"
)

var provisionCmd = cli.Command{
	Name:  "provision",
	Usage: "Provision a new system",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "disk",
			Usage: "Disk to provision.  \"any\" to choose one.  Disk must be empty or be wiped.",
		},
		cli.BoolFlag{
			Name:  "wipe",
			Usage: "Wipe the chosen disk.",
		},
	},
	Action: doProvision,
}

func doProvision(ctx *cli.Context) error {
	if ctx.NArg() != 2 {
		return fmt.Errorf("Required arguments: certificate and key paths")
	}
	if ctx.String("disk") == "" {
		log.Warnf("No disk specified. No disk will be provisioned")
	}

	if !PathExists("/dev/tpm0") {
		return fmt.Errorf("No TPM.  No other subsystems have been implemented")
	}

	t, err := trust.NewTpm2()
	if err != nil {
		return err
	}
	defer t.Close()
	return t.Provision(ctx)
}

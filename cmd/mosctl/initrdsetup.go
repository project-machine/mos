package main

import (
	"fmt"

	"github.com/project-machine/mos/pkg/mosconfig"
	"github.com/project-machine/trust/pkg/trust"
	"github.com/urfave/cli"
)

var initrdSetupCmd = cli.Command{
	Name:   "initrd-setup",
	Usage:  "Setup a provisioned system for boot",
	Action: doInitrdSetup,
}

func doInitrdSetup(ctx *cli.Context) error {
	if !mosconfig.PathExists("/dev/tpm0") {
		return fmt.Errorf("No TPM.  No other subsystems have been implemented")
	}

	t, err := trust.NewTpm2()
	if err != nil {
		return err
	}
	defer t.Close()
	return t.InitrdSetup()
}

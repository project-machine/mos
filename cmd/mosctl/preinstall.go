package main

import (
	"fmt"

	"github.com/project-machine/mos/pkg/mosconfig"
	"github.com/project-machine/trust/pkg/trust"
	"github.com/urfave/cli"
)

var preInstallCmd = cli.Command{
	Name:   "preinstall",
	Usage:  "Create and commit new OS key before install",
	Action: doPreInstall,
}

func doPreInstall(ctx *cli.Context) error {
	if !mosconfig.PathExists("/dev/tpm0") {
		return fmt.Errorf("No TPM.  No other subsystems have been implemented")
	}

	t, err := trust.NewTpm2()
	if err != nil {
		return err
	}
	defer t.Close()

	return t.PreInstall()
}

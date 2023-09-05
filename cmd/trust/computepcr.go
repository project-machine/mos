package main

import (
	"errors"
	"fmt"

	"github.com/project-machine/mos/pkg/trust"
	"github.com/urfave/cli"
)

var computePCR7Cmd = cli.Command{
	Name:      "computePCR7",
	Usage:     "Compute PCR7 value for a given keyset",
	ArgsUsage: "<keyset-name>",
	Action:    doComputePCR7,
}

func doComputePCR7(ctx *cli.Context) error {
	args := ctx.Args()
	if len(args) != 1 {
		return errors.New("Required argument: keysetName")
	}
	keysetName := args[0]
	if keysetName == "" {
		return errors.New("Please specify a keyset name")
	}
	pcr7prod, pcr7lim, pcr7tpm, err := trust.ComputePCR7(keysetName)
	if err != nil {
		return fmt.Errorf("Failed to generate pcr7 values for %s keyset: (%w)\n", keysetName, err)
	}
	fmt.Printf("uki-production: %x\nuki-limited: %x\nuki-tpm: %x\n", pcr7prod, pcr7lim, pcr7tpm)

	return nil
}

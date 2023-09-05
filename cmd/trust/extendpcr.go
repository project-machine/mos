package main

import (
	"github.com/project-machine/mos/pkg/trust"
	"github.com/urfave/cli"
)

var extendPCR7Cmd = cli.Command{
	Name:   "extend-pcr7",
	Usage:  "Extend TPM PCR7",
	Action: doTpmExtend,
}

func doTpmExtend(ctx *cli.Context) error {
	t, err := trust.NewTpm2()
	if err != nil {
		return err
	}
	defer t.Close()

	return t.ExtendPCR7()
}

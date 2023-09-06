package main

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/project-machine/mos/pkg/mosconfig"
	"github.com/urfave/cli"
)

var mkProvisionCmd = cli.Command{
	Name:   "mkprovision",
	Usage:  "build a provisioning iso",
	Action: doMkProvision,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "project",
			Usage: "trust project (e.g. snakeoil:default) whose shim and kernel to use",
		},
		cli.StringFlag{
			Name:  "out,outfile,iso",
			Usage: "ISO file to create",
			Value: "provision.iso",
		},
	},
}

func doMkProvision(ctx *cli.Context) error {
	outfile := ctx.String("outfile")

	if !ctx.IsSet("project") {
		return fmt.Errorf("A project must be specified.  Try 'snakeoil:default' if you're just playing")
	}

	fullproject := ctx.String("project")
	s := strings.SplitN(fullproject, ":", 2)
	if len(s) != 2 {
		return fmt.Errorf("First argument must be keyset:project")
	}
	keyset := s[0]
	project := s[1]

	if err := mosconfig.BuildProvisioner(keyset, project, outfile); err != nil {
		return errors.Wrapf(err, "Failed building provisioning ISO")
	}

	return nil
}

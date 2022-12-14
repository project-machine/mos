package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli"
)

var installCmd = cli.Command{
	Name: "install",
	Usage: "install a new mos system",
	Action: doInstall,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name: "config-dir,c",
			Usage: "Directory where mos config is found",
			Value: "/config",
		},
		cli.StringFlag{
			Name: "scratchwrites-dir,s",
			Usage: "Directory where overlay scratch writes should be mounted",
			Value: "/scratch-writes",
		},
		cli.StringFlag{
			Name: "atomfs-store,a",
			Usage: "Directory under which atomfs store is kept",
			Value: "/atomfs-store",
		},
	},
}

func PathExists(d string) bool {
	_, err := os.Stat(d)
	if err != nil && os.IsNotExist(err) {
		return false
	}
	return true
}

func doInstall(ctx *cli.Context) error {
	// Expect config, scratch-writes, and atomfs-store to exist
	if !PathExists(ctx.String("atomfs-store")) {
		return fmt.Errorf("atomfs store not found")
	}

	if !PathExists(ctx.String("config-dir")) {
		return fmt.Errorf("mos config not found")
	}

	if !PathExists(ctx.String("scratchwrites-dir")) {
		return fmt.Errorf("Scratchwrites directory not found")
	}

	return nil
}

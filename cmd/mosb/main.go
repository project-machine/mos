package main

import (
	"fmt"
	"os"

	"github.com/apex/log"
	"github.com/project-machine/mos/pkg/mosconfig"
	"github.com/urfave/cli"
	"gopkg.in/yaml.v2"
)

func main() {
	app := cli.NewApp()
	app.Name = "mosb"
	app.Version = mosconfig.Version
	app.Commands = []cli.Command{
		manifestCmd,
		mkBootCmd,
		mkProvisionCmd,
		readSpec,
	}
	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:  "debug",
			Usage: "display additional debug information",
		},
	}

	app.Before = func(c *cli.Context) error {
		if c.Bool("debug") {
			log.SetLevel(log.DebugLevel)
		}
		return nil
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatalf("%v\n", err)
	}
}

var readSpec = cli.Command{
	Name:   "readspec",
	Usage:  "read a manifest.yaml and print out resulting struct",
	Action: doReadSpec,
	Hidden: true,
	UsageText: `in-file
		  in-file: file to read`,
}

func doReadSpec(ctx *cli.Context) error {
	args := ctx.Args()
	if len(args) < 1 {
		return fmt.Errorf("input file is a required positional argument")
	}

	bytes, err := os.ReadFile(args[0])
	if err != nil {
		return err
	}
	var manifest mosconfig.ImportFile
	if err = yaml.Unmarshal(bytes, &manifest); err != nil {
		return err
	}
	fmt.Printf("result: %#v", manifest)
	return nil
}

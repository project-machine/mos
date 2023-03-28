package main

import (
	"os"

	"github.com/apex/log"
	"github.com/urfave/cli"
)

const Version = "0.1"

func main() {
	app := cli.NewApp()
	app.Name = "mosb"
	app.Version = Version
	app.Commands = []cli.Command{
		manifestCmd,
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

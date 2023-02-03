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
		sociCmd,
	}
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "xxx",
			Usage: "xxx",
			Value: "x",
		},
		cli.BoolFlag{
			Name:  "debug",
			Usage: "display additional debug information",
		},
	}

	app.Before = func(c *cli.Context) error {
		if c.Bool("debug") {
			log.SetLevel(log.DebugLevel)
		}
		log.Infof("xxx is set: %v", c.IsSet("xxx"))
		log.Infof("value of xxx is: %v", c.String("xxx"))
		return nil
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatalf("%v\n", err)
	}
}

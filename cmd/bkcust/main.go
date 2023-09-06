package main

import (
	"log"
	"os"

	cli "github.com/urfave/cli/v2"
)

func main() {

	app := cli.NewApp()
	app.Name = "bkcust"
	app.Usage = "Create customized artifacts from a bootkit"
	app.Version = "0.0.1"
	app.Commands = []*cli.Command{
		&initrdCmd,
		&shimCmd,
		&signEfiCmd,
		//&stubbyCmd,
		&virtFwCmd,
	}
	app.Flags = []cli.Flag{
		&cli.BoolFlag{
			Name:  "debug",
			Usage: "display additional debug information on stderr",
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatalf("%v\n", err)
	}
}

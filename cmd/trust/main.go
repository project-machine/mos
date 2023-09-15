package main

import (
	"os"

	"github.com/apex/log"
	"github.com/project-machine/mos/pkg/trust"
	"github.com/urfave/cli"
)

//   provision - dangerous
//   boot - read data from tpm, extend pcr7
//   intrd-setup - create new luks key, extend pcr7

func main() {
	app := cli.NewApp()
	app.Name = "trust"
	app.Usage = "Manage the trustroot"
	app.Version = trust.Version
	app.Commands = []cli.Command{
		initrdSetupCmd,
		preInstallCmd,
		provisionCmd,
		tpmPolicyGenCmd,
		extendPCR7Cmd,
		computePCR7Cmd,

		// keyset
		keysetCmd,

		// launch
		launchCmd,

		// project
		projectCmd,

		// sudi
		sudiCmd,

		// sign
		signCmd,

		// verify
		verifyCmd,
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

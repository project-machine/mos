package main

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/project-machine/mos/pkg/trust"
	"github.com/urfave/cli"
)

var signCmd = cli.Command{
	Name:  "sign",
	Usage: "Create Digital Signature",
	Subcommands: []cli.Command{
		cli.Command{
			Name:      "efi",
			Action:    doSignEFI,
			Usage:     "sign an efi binary",
			ArgsUsage: "<efi-file>",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "key",
					Usage: "The private key to sign the efi binary.",
				},
				cli.StringFlag{
					Name:  "cert",
					Usage: "The X509 certificate for creating signature.",
				},
				cli.StringFlag{
					Name:  "output",
					Usage: "PathName for the signed efi binary.",
				},
			},
		},
	},
}

func doSignEFI(ctx *cli.Context) error {
	args := ctx.Args()
	if len(args) != 1 {
		return errors.New("The pathname of the efi binary is missing (please see \"--help\")")
	}

	efibinary := args[0]
	if efibinary == "" {
		return errors.New("Please specify the efi binary")
	}

	// Make sure efibinary exists
	if !PathExists(efibinary) {
		return fmt.Errorf("%s does not exist", efibinary)
	}

	key := ctx.String("key")
	cert := ctx.String("cert")
	output := ctx.String("output")

	if key == "" || cert == "" || output == "" {
		return errors.New("Specify a key, cert and output. (please see \"--help\")")
	}

	// Make sure the key and cert exists
	if !PathExists(key) {
		return fmt.Errorf("%s does not exist", key)
	}

	if !PathExists(cert) {
		return fmt.Errorf("%s does not exist", cert)
	}

	// Sign the binary
	err := trust.SignEFI(efibinary, output, key, cert)
	if err != nil {
		return err
	}
	fmt.Printf("Signed %s\n", output)
	return nil
}

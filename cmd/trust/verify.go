package main

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/project-machine/mos/pkg/trust"
	"github.com/urfave/cli"
)

var verifyCmd = cli.Command{
	Name:  "verify",
	Usage: "Verify a digital Signature",
	Subcommands: []cli.Command{
		cli.Command{
			Name:      "efi",
			Action:    doVerifyEFI,
			Usage:     "verify a signed efi binary",
			ArgsUsage: "<signed-efi-file>",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "cert",
					Usage: "The X509 certificate to verify signature.",
				},
			},
		},
	},
}

func doVerifyEFI(ctx *cli.Context) error {
	args := ctx.Args()
	if len(args) != 1 {
		return errors.New("The pathname of the signed efi binary is missing (please see \"--help\")")
	}

	signedefibinary := args[0]
	if signedefibinary == "" {
		return errors.New("Please specify the signed efi binary")
	}

	// Make sure efibinary exists
	if !PathExists(signedefibinary) {
		return fmt.Errorf("%s does not exist", signedefibinary)
	}

	cert := ctx.String("cert")

	if cert == "" {
		return errors.New("Specify a cert to verify signature.")
	}

	// Make sure the cert exists
	if !PathExists(cert) {
		return fmt.Errorf("%s does not exist", cert)
	}

	// Verify the binary
	verified, err := trust.VerifyEFI(cert, signedefibinary)
	if err != nil {
		return err
	}
	if !verified {
		fmt.Printf("Signature verification failed")
		return errors.New("Signature verification failed")
	} else {
		fmt.Printf("Signature verification OK\n")
		return nil
	}
}

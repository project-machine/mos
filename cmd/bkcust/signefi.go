package main

import (
	"fmt"
	"os"

	"github.com/foxboron/go-uefi/efi/pecoff"
	"github.com/project-machine/mos/pkg/cert"
	cli "github.com/urfave/cli/v2"
)

var signEfiCmd = cli.Command{
	Name:      "sign-efi",
	Action:    doSignEfi,
	ArgsUsage: "app.efi cert.pem key.pem",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "output, o",
			Usage: "Put modified shim in <output>",
			Value: "",
		},
	},
}

func doSignEfi(ctx *cli.Context) error {
	args := ctx.Args().Slice()
	if len(args) != 3 {
		return fmt.Errorf("Got %d args, expected 3", len(args))
	}

	efiFile := args[0]
	certFile := args[1]
	keyFile := args[2]

	output := ctx.String("output")
	if output == "" {
		output = efiFile
	}

	signPKey, err := cert.KeyFromPemFile(keyFile)
	if err != nil {
		return fmt.Errorf("failed reading private key from %s: %v", keyFile, err)
	}

	signCert, err := cert.CertFromPemFile(certFile)
	if err != nil {
		return fmt.Errorf("failed reading cert from %s: %v", certFile, err)
	}

	peFile, err := os.ReadFile(efiFile)
	if err != nil {
		return fmt.Errorf("Failed reading efi '%s': %v", efiFile, err)
	}

	pctx := pecoff.PECOFFChecksum(peFile)

	sig, err := pecoff.CreateSignature(pctx, signCert, signPKey)
	if err != nil {
		return fmt.Errorf("Failed createsig:%v", err)
	}

	signedBin, err := pecoff.AppendToBinary(pctx, sig)
	if err != nil {
		return fmt.Errorf("Failed append :%v", err)
	}

	return os.WriteFile(output, signedBin, 0x755)
}

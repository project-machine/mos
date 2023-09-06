package main

import (
	"fmt"
	"strings"

	efi "github.com/canonical/go-efilib"
	"github.com/project-machine/mos/pkg/cert"
	"github.com/project-machine/mos/pkg/shim"
	"github.com/project-machine/mos/pkg/util"
	cli "github.com/urfave/cli/v2"
)

var shimCmd = cli.Command{
	Name: "shim",
	Subcommands: []*cli.Command{
		&cli.Command{
			Name:      "set-db",
			ArgsUsage: "shim.efi guid:cert [keydir:path/to/dir/ ...]",
			Action:    doSetDB,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:    "output",
					Aliases: []string{"o"},
					Usage:   "Put modified shim in <output>",
					Value:   "",
				},
			},
		},
	},
}

func doSetDB(ctx *cli.Context) error {
	args := ctx.Args().Slice()
	if len(args) < 1 {
		return fmt.Errorf("Got %d args, require 1 or more", len(args))
	}
	shimEfi := args[0]
	guidCerts := args[1:]

	if !PathExists(shimEfi) {
		return fmt.Errorf("shim '%s' does not exist", shimEfi)
	}

	if output := ctx.String("output"); output != "" {
		if err := util.CopyFileContents(shimEfi, output); err != nil {
			return fmt.Errorf("Failed to copy %s -> %s: %v", shimEfi, output, err)
		}
		shimEfi = output
	}

	// guidCerts are either <uuid>:*.pem or a "keydir" expected to
	// have a 'guid' and 'cert.pem' file
	certPath := ""
	sigDatas := []*efi.SignatureData{}
	for _, p := range guidCerts {
		if IsDir(p) {
			sd, err := cert.LoadSignatureDataDir(p)
			if err != nil {
				return fmt.Errorf("guidCert arg %s is a dir: %s", p, err)
			}
			sigDatas = append(sigDatas, sd)
		} else {
			toks := strings.Split(p, ":")
			if len(toks) < 2 {
				return fmt.Errorf("guidCert arg %s was not uuid:path", p)
			}
			guid, err := efi.DecodeGUIDString(toks[0])
			if err != nil {
				return fmt.Errorf("first token in guidCert '%s' not a valild uuid: %v", toks[0], err)
			}
			cert, err := cert.CertFromPemFile(certPath)
			if err != nil {
				return fmt.Errorf("Failed reading cert from %s: %s", p, err)
			}
			sigDatas = append(sigDatas, &efi.SignatureData{Owner: guid, Data: cert.Raw})
		}
	}

	return shim.SetVendorDB(shimEfi, cert.NewEFISignatureDatabase(sigDatas),
		cert.NewEFISignatureDatabase([]*efi.SignatureData{}))
}

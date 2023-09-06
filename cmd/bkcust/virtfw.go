package main

import (
	"fmt"
	"os"
	"strings"

	efi "github.com/canonical/go-efilib"
	"github.com/project-machine/mos/pkg/cert"
	"github.com/project-machine/mos/pkg/firmware"
	"github.com/project-machine/mos/pkg/util"
	cli "github.com/urfave/cli/v2"
)

var virtFwCmd = cli.Command{
	Name: "virtfw",
	Subcommands: []*cli.Command{
		&cli.Command{
			Name:      "secure-boot",
			ArgsUsage: "ovmf-vars.fd",
			Action:    doVirtFW,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "output, o",
					Usage: "Put modified vars in <output>",
					Value: "",
				},
				&cli.StringFlag{
					Name:    "platform",
					Aliases: []string{"pk"},
					Usage:   "Platform key",
					Value:   "",
				},
				&cli.StringSliceFlag{
					Name:  "kek",
					Usage: "key exchange key",
					Value: &cli.StringSlice{},
				},
				&cli.StringSliceFlag{
					Name:  "db",
					Usage: "db key",
					Value: &cli.StringSlice{},
				},
				&cli.StringSliceFlag{
					Name:  "mok",
					Usage: "mok key",
					Value: &cli.StringSlice{},
				},
			},
		},
	},
}

// guidCerts are either <uuid>:*.pem or a "keydir" expected to
// have a 'guid' and 'cert.pem' file
func readGuidCertString(guidCerts []string) ([]*efi.SignatureData, error) {
	certPath := ""
	sigDatas := []*efi.SignatureData{}
	for _, p := range guidCerts {
		if IsDir(p) {
			sd, err := cert.LoadSignatureDataDir(p)
			if err != nil {
				return sigDatas, fmt.Errorf("guidCert arg %s is a dir: %s", p, err)
			}
			sigDatas = append(sigDatas, sd)
		} else {
			toks := strings.Split(p, ":")
			if len(toks) < 2 {
				return sigDatas, fmt.Errorf("guidCert arg %s was not uuid:path", p)
			}
			guid, err := efi.DecodeGUIDString(toks[0])
			if err != nil {
				return sigDatas, fmt.Errorf("first token in guidCert '%s' not a valild uuid: %v", toks[0], err)
			}
			cert, err := cert.CertFromPemFile(certPath)
			if err != nil {
				return sigDatas, fmt.Errorf("Failed reading cert from %s: %s", p, err)
			}
			sigDatas = append(sigDatas, &efi.SignatureData{Owner: guid, Data: cert.Raw})
		}
	}

	return sigDatas, nil
}

func doVirtFW(ctx *cli.Context) error {
	var err error
	args := ctx.Args().Slice()
	if len(args) != 1 {
		return fmt.Errorf("Got %d args, require 1", len(args))
	}
	ovmfVarsIn := args[0]
	ovmfVarsOut := ctx.String("output")
	if ovmfVarsOut == "" {
		tmpf, err := os.CreateTemp("", "doVirtFW-")
		if err != nil {
			return fmt.Errorf("Failed to create tmpfile: %v\n", err)
		}
		ovmfVarsOut = tmpf.Name()
		tmpf.Close()
		defer os.Remove(ovmfVarsOut)
	}

	platformKey := efi.SignatureData{}
	var kekData, dbData, mokData []*efi.SignatureData

	if pkstr := ctx.String("platform"); pkstr != "" {
		sigdlist, err := readGuidCertString([]string{pkstr})
		if err != nil {
			return fmt.Errorf("Failed to read platform key: %v", err)
		}
		platformKey = *sigdlist[0]
	}

	if kekStrs := ctx.StringSlice("kek"); len(kekStrs) != 0 {
		kekData, err = readGuidCertString(kekStrs)
		if err != nil {
			return fmt.Errorf("Failed to read kek key: %v", err)
		}
	}

	if dbStrs := ctx.StringSlice("db"); len(dbStrs) != 0 {
		dbData, err = readGuidCertString(dbStrs)
		if err != nil {
			return fmt.Errorf("Failed to read db key: %v", err)
		}
	}

	if mokStrs := ctx.StringSlice("mok"); len(mokStrs) != 0 {
		mokData, err = readGuidCertString(mokStrs)
		if err != nil {
			return fmt.Errorf("Failed to read mok key: %v", err)
		}
	}

	err = firmware.OVMFPopulateSecureBoot(
		ovmfVarsIn, ovmfVarsOut, &platformKey, kekData, dbData, mokData)
	if err != nil {
		return fmt.Errorf("Failed to populate %s from %s: %v", ovmfVarsIn, ovmfVarsOut, err)
	}

	if out := ctx.String("output"); out == "" {
		if err := util.CopyFileContents(ovmfVarsOut, ovmfVarsIn); err != nil {
			return fmt.Errorf("Failed to copy tmp file %s -> %s: %v", ovmfVarsOut, ovmfVarsIn, err)
		}
	}

	fmt.Fprintf(os.Stderr, "Wrote to %s\n", ovmfVarsOut)

	return nil
}

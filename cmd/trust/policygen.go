package main

import (
	"errors"
	"os"

	"github.com/project-machine/mos/pkg/trust"
	"github.com/urfave/cli"
)

var tpmPolicyGenCmd = cli.Command{
	Name:   "tpm-policy-gen",
	Usage:  "Generate tpm policy for a keyset",
	Action: doTpmPolicygen,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "pf, passwd-policy-file",
			Usage: "File to which to write password policy",
			Value: "passwd_policy.out",
		},
		cli.StringFlag{
			Name:  "lf, luks-policy-file",
			Usage: "File to which to write luks policy",
			Value: "luks_policy.out",
		},
		cli.StringFlag{
			Name:  "pcr7-tpm",
			Usage: "File from which to read uki-tpm pcr7 value",
		},
		cli.StringFlag{
			Name:  "pcr7-production",
			Usage: "File from which to read uki-production pcr7 value",
		},
		cli.StringFlag{
			Name:  "pv, policy-version",
			Usage: "A four digit policy version, i.e. 0001",
			Value: "0001",
		},
	},
}

func doTpmPolicygen(ctx *cli.Context) error {
	args := ctx.Args()
	if len(args) != 0 {
		return errors.New("Usage: extra arguments")
	}

	// Check inputs
	pcr7ProdFile := ctx.String("pcr7-production")
	pcr7TpmFile := ctx.String("pcr7-tpm")
	luksOutFile := ctx.String("luks-policy-file")
	passwdOutFile := ctx.String("passwd-policy-file")
	policyVersion := ctx.String("policy-version")

	if pcr7ProdFile == "" || pcr7TpmFile == "" {
		return errors.New("Missing pcr7 file(s).")
	}

	if luksOutFile == "" {
		luksOutFile = "luks_policy.out"
	}

	if passwdOutFile == "" {
		passwdOutFile = "passwd_policy.out"
	}

	// Read the pcr7 values from the specified files
	luksPcr7, err := os.ReadFile(pcr7ProdFile)
	if err != nil {
		return err
	}
	tpmpassPcr7, err := os.ReadFile(pcr7TpmFile)
	if err != nil {
		return err
	}

	// Generate the TPM EA Policies and Write them to specified output files
	passwdPolDigest, err := trust.GenPasswdPolicy(tpmpassPcr7)
	if err != nil {
		return err
	}
	err = os.WriteFile(passwdOutFile, passwdPolDigest, 0400)
	if err != nil {
		return err
	}

	// Remove first policy file if an error occurs with second one
	defer func() {
		if err != nil {
			os.Remove(passwdOutFile)
		}
	}()

	luksPolDigest, err := trust.GenLuksPolicy(luksPcr7, policyVersion)
	if err != nil {
		return err
	}
	err = os.WriteFile(luksOutFile, luksPolDigest, 0400)
	if err != nil {
		return err
	}

	return nil
}

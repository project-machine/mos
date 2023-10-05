package main

import (
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/apex/log"
	"github.com/go-git/go-git/v5"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/project-machine/mos/pkg/mosconfig"
	tree "github.com/project-machine/mos/pkg/printdirtree"
	"github.com/project-machine/mos/pkg/trust"
	"github.com/project-machine/mos/pkg/utils"
	"github.com/urfave/cli"
)

var KeysetKeyDirs = []string{
	"manifest",
	"manifest-ca",
	"pcr7data",
	"sudi-ca",
	"tpmpol-admin",
	"tpmpol-luks",
	"uefi-db",
	"uefi-kek",
	"uefi-pk",
	"uki-limited",
	"uki-production",
	"uki-tpm",
}

const (
	middleSym = "├──"
	columnSym = "│"
	lastSym   = "└──"
	firstSym  = middleSym
)

func isValidKeyDir(keydir string) bool {
	for _, dir := range KeysetKeyDirs {
		if dir == keydir {
			return true
		}
	}
	if strings.HasPrefix(keydir, "manifest:") {
		return true
	}
	return false
}

func generateMosCreds(keysetPath string, ctemplate *x509.Certificate) error {
	type AddCertInfo struct {
		cn     string
		doguid bool
	}
	keyinfo := map[string]AddCertInfo{
		"tpmpol-admin":   {"TPM EAPolicy Admin", false},
		"tpmpol-luks":    {"TPM EAPolicy LUKS", false},
		"uki-tpm":        {"UKI TPM", true},
		"uki-limited":    {"UKI Limited", true},
		"uki-production": {"UKI Production", true},
		"uefi-db":        {"UEFI DB", true},
	}

	for key, CertInfo := range keyinfo {
		ctemplate.Subject.CommonName = CertInfo.cn
		err := generateCreds(filepath.Join(keysetPath, key), CertInfo.doguid, ctemplate)
		if err != nil {
			return err
		}
	}
	return nil
}

func makeKeydirs(keysetPath string) error {
	err := os.MkdirAll(keysetPath, 0750)
	if err != nil {
		return err
	}

	for _, dir := range KeysetKeyDirs {
		err = os.Mkdir(filepath.Join(keysetPath, dir), 0750)
		if err != nil {
			return err
		}
	}
	return nil
}

func initkeyset(keysetName string, Org []string) error {
	var caTemplate, certTemplate x509.Certificate
	const (
		doGUID = true
		noGUID = false
	)
	if keysetName == "" {
		return errors.New("keyset parameter is missing")
	}

	moskeysetPath, err := utils.GetMosKeyPath()
	if err != nil {
		return err
	}
	keysetPath := filepath.Join(moskeysetPath, keysetName)
	if utils.PathExists(keysetPath) {
		return fmt.Errorf("%s keyset already exists", keysetName)
	}

	os.MkdirAll(keysetPath, 0750)

	// Start generating the new keys
	defer func() {
		if err != nil {
			os.RemoveAll(keysetPath)
		}
	}()

	err = makeKeydirs(keysetPath)
	if err != nil {
		return err
	}

	// Prepare certificate template

	caTemplate.Subject.Organization = Org
	caTemplate.Subject.OrganizationalUnit = []string{"Project Machine Project " + keysetName}
	caTemplate.Subject.CommonName = "Manifest rootCA"
	caTemplate.NotBefore = time.Now()
	caTemplate.NotAfter = time.Now().AddDate(25, 0, 0)
	caTemplate.IsCA = true
	caTemplate.BasicConstraintsValid = true

	// Generate the manifest rootCA
	err = generaterootCA(filepath.Join(keysetPath, "manifest-ca"), &caTemplate, noGUID)
	if err != nil {
		return err
	}

	// Generate the sudi rootCA
	caTemplate.Subject.CommonName = "SUDI rootCA"
	caTemplate.NotAfter = time.Date(2099, time.December, 31, 23, 0, 0, 0, time.UTC)
	err = generaterootCA(filepath.Join(keysetPath, "sudi-ca"), &caTemplate, noGUID)
	if err != nil {
		return err
	}

	// Generate PK
	caTemplate.Subject.CommonName = "UEFI PK"
	caTemplate.NotAfter = time.Now().AddDate(50, 0, 0)
	err = generaterootCA(filepath.Join(keysetPath, "uefi-pk"), &caTemplate, doGUID)
	if err != nil {
		return err
	}

	// Generate additional MOS credentials
	certTemplate.Subject.Organization = Org
	certTemplate.Subject.OrganizationalUnit = []string{"Project Machine Project " + keysetName}
	certTemplate.NotBefore = time.Now()
	certTemplate.NotAfter = time.Now().AddDate(25, 0, 0)
	certTemplate.KeyUsage = x509.KeyUsageDigitalSignature
	certTemplate.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning}

	err = generateMosCreds(keysetPath, &certTemplate)
	if err != nil {
		return err
	}

	// Generate KEK, signed by PK
	CAcert, CAprivkey, err := getCA("uefi-pk", keysetName)
	if err != nil {
		return err
	}
	// reuse certTemplate with some modifications
	certTemplate.Subject.CommonName = "UEFI KEK"
	certTemplate.NotAfter = time.Now().AddDate(50, 0, 0)
	certTemplate.ExtKeyUsage = nil
	err = SignCert(&certTemplate, CAcert, CAprivkey, filepath.Join(keysetPath, "uefi-kek"))
	if err != nil {
		return err
	}
	guid := uuid.NewString()
	err = os.WriteFile(filepath.Join(keysetPath, "uefi-kek", "guid"), []byte(guid), 0640)
	if err != nil {
		return err
	}

	// Generate sample uuid, manifest key and cert
	mName := filepath.Join(keysetPath, "manifest", "default")
	if err = utils.EnsureDir(mName); err != nil {
		return errors.Wrapf(err, "Failed creating default project directory")
	}
	sName := filepath.Join(mName, "sudi")
	if err = utils.EnsureDir(sName); err != nil {
		return errors.Wrapf(err, "Failed creating default sudi directory")
	}

	if err = generateNewUUIDCreds(keysetName, mName); err != nil {
		return errors.Wrapf(err, "Failed creating default project keyset")
	}

	// Generate the 3 uki-* pcr7 values for this keyset
	prodPcr, limitedPcr, tpmPcr, err := trust.ComputePCR7(keysetName)
	if err != nil {
		return err
	}

	// Generate the luks EA Policies for this keyset
	luksPolicyDigest, err := trust.GenLuksPolicy(prodPcr, trust.PolicyVersion.String())
	if err != nil {
		return err
	}

	// Generate the tpm passwod EA Policy Digest for this keyset
	tpmpasswdPolicyDigest, err := trust.GenPasswdPolicy(tpmPcr)
	if err != nil {
		return err
	}

	// Add the signdata to the keyset
	p := pcr7Data{
		limited:            limitedPcr,
		tpm:                tpmPcr,
		production:         prodPcr,
		passwdPolicyDigest: tpmpasswdPolicyDigest,
		luksPolicyDigest:   luksPolicyDigest}

	if err = addPcr7data(keysetName, p); err != nil {
		return fmt.Errorf("Failed to add the pcr7data to keyset %q: (%w)", keysetName, err)
	}

	return nil
}

var keysetCmd = cli.Command{
	Name:  "keyset",
	Usage: "Administer keysets for mos",
	Subcommands: []cli.Command{
		{
			Name:   "list",
			Action: doListKeysets,
			Usage:  "list keysets",
		},
		{
			Name:      "add",
			Action:    doAddKeyset,
			Usage:     "add a new keyset",
			ArgsUsage: "<keyset-name>",
			Flags: []cli.Flag{
				cli.StringSliceFlag{
					Name:  "org, Org, organization",
					Usage: "X509-Organization field to add to certificates when generating a new keyset. (optional)",
				},
				cli.StringFlag{
					Name:  "bootkit-version",
					Usage: "Version of bootkit artifacts to use",
					Value: trust.BootkitVersion,
				},
				cli.StringFlag{
					Name:   "mosctl-path",
					Usage:  "A path to a custom mosctl binary to insert",
					Hidden: true,
					Value:  "",
				},
			},
		},
		{
			Name:      "show",
			Action:    doShowKeyset,
			Usage:     "show keyset key values or paths",
			ArgsUsage: "<keyset-name> <key> [<item>]",
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "path",
					Usage: "Show path only to keyset key item",
				},
				cli.BoolFlag{
					Name:  "value",
					Usage: "Show value only of keyset key item",
				},
			},
		},
		{
			Name:      "pcr7data",
			Action:    doAddPCR7data,
			Usage:     "include the specified pcr7data into keyset",
			ArgsUsage: "<keyset-name>",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "pcr7-tpm",
					Usage: "Pathname to the pcr7 tpm binary file",
				},
				cli.StringFlag{
					Name:  "pcr7-limited",
					Usage: "Pathname to the pcr7 limited binary file",
				},
				cli.StringFlag{
					Name:  "pcr7-prod",
					Usage: "Pathname to the pcr7 production binary file",
				},
				cli.StringFlag{
					Name:  "passwdPolicy",
					Usage: "Pathname to the tpm passwd policy file (optional)",
					Value: "passwd_policy.out",
				},
				cli.StringFlag{
					Name:  "luksPolicy",
					Usage: "Pathname to the luks policy file (optional)",
					Value: "luks_policy.out",
				},
			},
		},
	},
}

func doAddKeyset(ctx *cli.Context) error {
	args := ctx.Args()
	if len(args) != 1 {
		return errors.New("A name for the new keyset is required (please see \"--help\")")
	}

	keysetName := args[0]
	if keysetName == "" {
		return errors.New("Please specify keyset name")
	}

	mosctlPath := ctx.String("mosctl-path")

	bootkitVersion := ctx.String("bootkit-version")
	Org := ctx.StringSlice("org")
	if Org == nil {
		log.Infof("X509-Organization field for new certificates not specified.")
	}

	// See if keyset exists
	mosKeyPath, err := utils.GetMosKeyPath()
	if err != nil {
		return err
	}

	keysetPath := filepath.Join(mosKeyPath, keysetName)
	if utils.PathExists(keysetPath) {
		return fmt.Errorf("%s keyset already exists", keysetName)
	}

	switch keysetName {
	case "snakeoil":
		// git clone if keyset is snakeoil
		_, err = git.PlainClone(keysetPath, false, &git.CloneOptions{URL: "https://github.com/project-machine/keys.git"})

	default:
		// Otherwise, generate a new keyset
		err = initkeyset(keysetName, Org)
	}
	if err != nil {
		os.Remove(keysetPath)
		return errors.Wrapf(err, "Failed creating keyset %q", keysetName)
	}

	// Now create the bootkit artifacts
	if err = trust.SetupBootkit(keysetName, bootkitVersion, mosctlPath); err != nil {
		return fmt.Errorf("Failed creating bootkit artifacts for keyset %q: (%w)", keysetName, err)
	}

	if err := buildProvisioner(keysetName); err != nil {
		return errors.Wrapf(err, "Failed to create provisioning ISO")
	}

	if err := buildInstaller(keysetName); err != nil {
		return errors.Wrapf(err, "Failed to create provisioning ISO")
	}

	return nil
}

func doListKeysets(ctx *cli.Context) error {
	if len(ctx.Args()) != 0 {
		return fmt.Errorf("Wrong number of arguments (please see \"--help\")")
	}
	moskeysetPath, err := utils.GetMosKeyPath()
	if err != nil {
		return err
	}
	dirs, err := os.ReadDir(moskeysetPath)
	if err != nil {
		return fmt.Errorf("Failed reading keys directory %q: %w", moskeysetPath, err)
	}

	for _, keyname := range dirs {
		fmt.Printf("%s\n", keyname.Name())
	}

	return nil
}

func doShowKeyset(ctx *cli.Context) error {
	if len(ctx.Args()) == 0 {
		return fmt.Errorf("Please specify keyset name. Select from 'trust keyset list'")
	}

	keysetName := ctx.Args()[0]
	if keysetName == "" {
		return fmt.Errorf("Please specify keyset name. Select from 'trust keyset list'")
	}

	moskeysetPath, err := utils.GetMosKeyPath()
	if err != nil {
		return err
	}

	keysetPath := filepath.Join(moskeysetPath, keysetName)
	if !utils.PathExists(keysetPath) {
		return fmt.Errorf("Unknown keyset '%s', cannot find keyset at path: %q", keysetName, keysetPath)
	}

	// no keyset key name specified only, print path if --path, otherwise list all key dir names
	if len(ctx.Args()) < 2 {
		if ctx.Bool("path") {
			fmt.Printf("%s\n", keysetPath)
			for _, keyDir := range KeysetKeyDirs {
				keyPath := filepath.Join(keysetPath, keyDir)
				fmt.Printf("%s\n", keyPath)
			}
		} else {

			if err := tree.PrintDirs(keysetPath, KeysetKeyDirs); err != nil {
				return err
			}
		}
		return nil
	}

	keyName := ctx.Args()[1]
	if keyName == "" {
		return fmt.Errorf("Please specify keyset key name, must be one of: %s", strings.Join(KeysetKeyDirs, ", "))
	}

	if !isValidKeyDir(keyName) {
		return fmt.Errorf("Invalid keyset key name '%s':, must be one of: %s", strings.Join(KeysetKeyDirs, ", "))
	}

	// manifest requires a project name
	if keyName == "manifest" {
		return fmt.Errorf("keyset key 'manifest' requires a project value, use 'trust project list %s' to list projects for this keyset", keysetName)
	}

	if strings.HasPrefix(keyName, "manifest:") {
		keyName = strings.Replace(keyName, ":", "/", 1)
	}

	keyPath := filepath.Join(keysetPath, keyName)
	if !utils.PathExists(keyPath) {
		return fmt.Errorf("Keyset %s key %q does not exist at %q", keysetName, keyName, keyPath)
	}

	if len(ctx.Args()) > 2 {
		item := ctx.Args()[2]
		fullPath := filepath.Join(keyPath, item)
		if !utils.PathExists(fullPath) {
			return fmt.Errorf("Failed reading keyset %s key %s item %s at %q: %w", keysetName, keyName, item, fullPath, err)
		}

		contents, err := os.ReadFile(fullPath)
		if err != nil {
			return fmt.Errorf("Failed reading keyset %s key %s item %s at %q: %w", keysetName, keyName, item, fullPath, err)
		}
		if ctx.Bool("path") {
			fmt.Println(fullPath)
		} else if ctx.Bool("value") {
			fmt.Printf("%s", string(contents))
		} else {
			fmt.Printf("%s\n%s\n", fullPath, string(contents))
		}
		return nil
	}

	// no item specified, crawl dir and print contents or path
	keyFiles, err := os.ReadDir(keyPath)
	if err != nil {
		return fmt.Errorf("keyset %s key %s directory %q: %w", keysetName, keyName, keyPath, err)
	}

	printPath := ctx.Bool("path")
	for _, dEntry := range keyFiles {
		if dEntry.IsDir() {
			continue
		}
		keyFile := dEntry.Name()
		fullPath := filepath.Join(keyPath, keyFile)
		contents, err := os.ReadFile(fullPath)
		if err != nil {
			return fmt.Errorf("Failed reading keyset %s key %s item %s at %q: %w", keysetName, keyName, keyFile, fullPath, err)
		}
		if printPath {
			fmt.Println(fullPath)
		} else {
			fmt.Printf("%s\n%s\n", fullPath, string(contents))
		}
	}

	return nil
}

func doAddPCR7data(ctx *cli.Context) error {
	args := ctx.Args()
	if len(args) != 1 {
		return errors.New("Missing arguments")
	}

	keysetName := args[0]

	limited := ctx.String("pcr7-limited")
	tpm := ctx.String("pcr7-tpm")
	prod := ctx.String("pcr7-prod")
	passwdPolicy := ctx.String("passwdPolicy")
	luksPolicy := ctx.String("luksPolicy")

	// Check inputs
	// Policy output files have defaults same as when generating policy
	if limited == "" || tpm == "" || prod == "" {
		return errors.New("PCR7 values are missing")
	}
	if !utils.PathExists(limited) || !utils.PathExists(tpm) || !utils.PathExists(prod) {
		return errors.New("Some PCR7 files do not exist")
	}
	if !utils.PathExists(passwdPolicy) || !utils.PathExists(luksPolicy) {
		return errors.New("Some policy digest files do not exist")
	}

	// Read the PCR values
	limitedPCR7, err := os.ReadFile(limited)
	if err != nil {
		return fmt.Errorf("Failed to read %q: (%w)", limited, err)
	}
	tpmPCR7, err := os.ReadFile(tpm)
	if err != nil {
		return fmt.Errorf("Failed to read %q: (%w)", tpm, err)
	}
	prodPCR7, err := os.ReadFile(prod)
	if err != nil {
		return fmt.Errorf("Failed to read %q: (%w)", prod, err)
	}

	// Read the Policy Digest Files
	passwdPD, err := os.ReadFile(passwdPolicy)
	if err != nil {
		return fmt.Errorf("Failed to read %q: (%w)", passwdPolicy, err)
	}
	luksPD, err := os.ReadFile(luksPolicy)
	if err != nil {
		return fmt.Errorf("Failed to read %q: (%w)", luksPolicy, err)
	}

	p := pcr7Data{
		limited:            limitedPCR7,
		tpm:                tpmPCR7,
		production:         prodPCR7,
		passwdPolicyDigest: passwdPD,
		luksPolicyDigest:   luksPD}

	if err := addPcr7data(keysetName, p); err != nil {
		return err
	}

	return nil
}

// Build a provisioning ISO for a new keyset.  We create it for the
// 'default' project.
func buildProvisioner(keysetName string) error {
	if runtime.GOARCH != "amd64" {
		log.Warnf("Running on %q, so not building bootkit artifacts (only amd64 supported).", runtime.GOARCH)
		return nil
	}

	moskeysetPath, err := utils.GetMosKeyPath()
	if err != nil {
		return err
	}
	keyPath := filepath.Join(moskeysetPath, keysetName)
	outfile := filepath.Join(keyPath, "artifacts", "provision.iso")
	utils.EnsureDir(filepath.Dir(outfile))

	if err := mosconfig.BuildProvisioner(keysetName, "default", outfile); err != nil {
		return errors.Wrapf(err, "Failed to create provisioning ISO")
	}

	log.Infof("Created %q", outfile)
	return nil
}

func buildInstaller(keysetName string) error {
	if runtime.GOARCH != "amd64" {
		log.Warnf("Running on %q, so not building bootkit artifacts (only amd64 supported).", runtime.GOARCH)
		return nil
	}

	moskeysetPath, err := utils.GetMosKeyPath()
	if err != nil {
		return err
	}
	keyPath := filepath.Join(moskeysetPath, keysetName)
	outfile := filepath.Join(keyPath, "artifacts", "install.iso")
	if err := utils.EnsureDir(filepath.Dir(outfile)); err != nil {
		return errors.Wrapf(err, "Failed creating %q", outfile)
	}

	if err := mosconfig.BuildInstaller(keysetName, "default", outfile); err != nil {
		return errors.Wrapf(err, "Failed to create provisioning ISO")
	}

	log.Infof("Created %q", outfile)
	return nil
}

package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/apex/log"
	"github.com/pkg/errors"
	"github.com/project-machine/mos/pkg/provider"
	"github.com/project-machine/mos/pkg/utils"
	"github.com/urfave/cli"
)

var launchCmd = cli.Command{
	Name:  "launch",
	Usage: "launch a new machine",
	UsageText: `name install-url
		  name: name to give the VM
		  install-url: install.json distoci URL to install (e.g. zothub.io/machine/zot/install:1.0.0)

		  Note that install is not yet supported.`,
	Action: doLaunch,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "project",
			Usage: "keyset:project to which this machine will belong (TRUST_PROJECT)",
		},
		cli.StringFlag{
			Name:  "serial, uuid",
			Usage: "Serial number UUID to assign to the machine, empty to use a random UUID",
			Value: "",
		},
		cli.BoolFlag{
			Name:  "debug",
			Usage: "show console during provision and install",
		},
		cli.BoolFlag{
			Name:  "skip-provisioning",
			Usage: "Skip provisioning the machine",
		},
		cli.BoolFlag{
			Name:  "skip-install",
			Usage: "Skip running the install ISO",
		},
		cli.StringFlag{
			Name:  "type",
			Usage: "Type of machine to launch.",
			Value: "kvm",
		},
	},
}

func splitFullProject(full string) (string, string, error) {
	s := strings.Split(full, ":")
	if len(s) != 2 {
		return "", "", errors.Errorf("Bad project name %q, should be keyset:project, e.g. snakeoil:default.", full)
	}
	return s[0], s[1], nil
}

func doLaunch(ctx *cli.Context) error {
	args := ctx.Args()
	if len(args) < 1 {
		return errors.New("A name for the new machine is required")
	}

	installUrl := ""
	if !ctx.Bool("skip-install") {
		if len(args) != 2 {
			return errors.New("Install manifest URL is required")
		}
		installUrl = args[1]
	}

	mtype := ctx.String("type")
	var p provider.Provider
	var err error
	switch mtype {
	case "kvm":
		p, err = provider.NewKVMProvider()
		if err != nil {
			return errors.Wrapf(err, "Failed to instantiate machine provider for type %q", mtype)
		}
	default:
		return errors.Errorf("Unknown machine type: %q", mtype)
	}

	mname := args[0]
	if mname == "" {
		return errors.New("Please specify machine name")
	}

	if p.Exists(mname) {
		return errors.Errorf("Machine %q already exists", mname)
	}

	fullProject := ctx.String("project")
	keyset, project, err := splitFullProject(fullProject)
	if err != nil {
		return err
	}

	trustDir, err := utils.GetMosKeyPath()
	if err != nil {
		return err
	}

	keysetDir := filepath.Join(trustDir, keyset)
	projDir := filepath.Join(keysetDir, "manifest", project)
	if !utils.PathExists(projDir) {
		return errors.Errorf("Project %s not found", fullProject)
	}

	uuid := ctx.String("uuid")
	sudiDir, err := genSudi(keysetDir, projDir, uuid)
	if err != nil {
		return errors.Wrapf(err, "Failed generating SUDI")
	}
	if uuid == "" {
		uuid = filepath.Base(sudiDir)
	}

	defer func() {
		if err != nil {
			os.RemoveAll(sudiDir)
		}
	}()

	if err := makeSudiVfat(sudiDir); err != nil {
		return errors.Wrapf(err, "Failed creating SUDI disk")
	}

	if err := makeInstallVFAT(sudiDir, installUrl); err != nil {
		return errors.Wrapf(err, "Failed creating SUDI disk")
	}

	m, err := p.New(mname, fullProject, uuid)
	if err != nil {
		return errors.Wrapf(err, "Failed to create new machine")
	}

	defer func() {
		if err != nil {
			p.Delete(mname)
		}
	}()

	if err := m.RunProvision(ctx.Bool("debug")); err != nil {
		return errors.Wrapf(err, "Failed to run provisioning ISO")
	}

	if installUrl == "" {
		log.Infof("Skipping install per user request")
		return nil
	}

	if err := m.RunInstall(ctx.Bool("debug")); err != nil {
		return errors.Wrapf(err, "Failed to run install ISO")
	}

	return nil
}

// Make a VFAT disk storing the already-generated SUDI cert and privkey
func makeSudiVfat(sudiDir string) error {
	cert := filepath.Join(sudiDir, "cert.pem")
	key := filepath.Join(sudiDir, "privkey.pem")
	disk := filepath.Join(sudiDir, "sudi.vfat")
	if !utils.PathExists(cert) || !utils.PathExists(key) {
		return errors.Errorf("cert or key does not exist")
	}
	if utils.PathExists(disk) {
		return errors.Errorf("sudi.vfat already exists")
	}

	f, err := os.OpenFile(disk, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return errors.Wrapf(err, "Failed creating sudi disk")
	}
	f.Close()

	if err := os.Truncate(disk, 20*1024*1024); err != nil {
		return errors.Wrapf(err, "Failed truncating sudi disk")
	}
	if err := utils.RunCommand("mkfs.vfat", "-n", "trust-data", disk); err != nil {
		return errors.Wrapf(err, "Failed formatting sudi disk")
	}
	if err := utils.RunCommand("mcopy", "-i", disk, cert, "::cert.pem"); err != nil {
		return errors.Wrapf(err, "Failed copying cert to sudi disk")
	}
	if err := utils.RunCommand("mcopy", "-i", disk, key, "::privkey.pem"); err != nil {
		return errors.Wrapf(err, "Failed copying key to sudi disk")
	}

	return nil
}

func makeInstallVFAT(sudiDir, url string) error {
	if url == "" {
		return nil
	}

	disk := filepath.Join(sudiDir, "install.vfat")
	if utils.PathExists(disk) {
		return errors.Errorf("%q already exists", disk)
	}

	f, err := os.CreateTemp("", "installfat")
	if err != nil {
		return err
	}
	urlfile := f.Name()
	defer os.Remove(urlfile)
	_, err = f.WriteString(url)
	f.Close()
	if err != nil {
		return err
	}

	f, err = os.OpenFile(disk, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return errors.Wrapf(err, "Failed creating install disk")
	}
	f.Close()

	if err = os.Truncate(disk, 20*1024*1024); err != nil {
		return errors.Wrapf(err, "Failed truncating install disk")
	}
	if err = utils.RunCommand("mkfs.vfat", "-n", "inst-data", disk); err != nil {
		return errors.Wrapf(err, "Failed formatting install disk")
	}
	if err = utils.RunCommand("mcopy", "-i", disk, urlfile, "::url.txt"); err != nil {
		return errors.Wrapf(err, "Failed copying urlfile to install disk")
	}

	return nil
}

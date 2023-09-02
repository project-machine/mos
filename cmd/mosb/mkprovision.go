package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/project-machine/mos/pkg/mosconfig"
	"github.com/urfave/cli"
)

var mkProvisionCmd = cli.Command{
	Name:   "mkprovision",
	Usage:  "build a provisioning iso",
	Action: doMkProvision,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "project",
			Usage: "trust project (e.g. snakeoil:default) whose shim and kernel to use",
		},
		cli.StringFlag{
			Name:  "out,outfile,iso",
			Usage: "ISO file to create",
			Value: "provision.iso",
		},
	},
}

// I'd like to make service_name be 'provision', but then the initrd would
// need to be updated, as it currently always does:
//    set -- mosctl $debug mount \
//        "--target=livecd" \
//        "--dest=$rootd" \
//        "${repo}/$name"

var manifestTemplate = `
version: 1
product: "%s"
update_type: complete
targets:
  - service_name: livecd
    source: "docker://zothub.io/machine/bootkit/provision-rootfs:%s-squashfs"
    version: %s
    service_type: fs-only
    nsgroup: "none"
    network:
      type: none
`

func doMkProvision(ctx *cli.Context) error {
	outfile := ctx.String("outfile")

	if !ctx.IsSet("project") {
		return fmt.Errorf("A project must be specified.  Try 'snakeoil:default' if you're just playing")
	}
	fullproject := ctx.String("project")
	s := strings.SplitN(fullproject, ":", 2)
	if len(s) != 2 {
		return fmt.Errorf("First argument must be keyset:project")
	}
	keyset := s[0]
	project := s[1]

	dir, err := os.MkdirTemp("", "provision")
	if err != nil {
		return errors.Wrapf(err, "failed creating temporary directory")
	}
	defer os.RemoveAll(dir)

	cacheDir := filepath.Join(dir, "cache")
	zotPort, cleanup, err := startZot(dir, cacheDir)
	if err != nil {
		return errors.Wrapf(err, "failed starting a local zot")
	}
	defer cleanup()

	repo := fmt.Sprintf("127.0.0.1:%d", zotPort)
	name := "machine/livecd:1.0.0"

	keyPath, err := mosconfig.MosKeyPath()
	if err != nil {
		return errors.Wrapf(err, "Failed finding mos key path")
	}
	pPath := filepath.Join(keyPath, keyset, "manifest", project, "uuid")
	productUUID, err := os.ReadFile(pPath)
	if err != nil {
		return errors.Wrapf(err, "Failed to read project uuid (%q)", pPath)
	}
	manifestText := fmt.Sprintf(manifestTemplate, string(productUUID), mosconfig.LayerVersion, mosconfig.LayerVersion)

	manifestpath := filepath.Join(dir, "manifest.yaml")
	err = os.WriteFile(manifestpath, []byte(manifestText), 0600)
	if err != nil {
		return errors.Wrapf(err, "failed writing the manifest file")
	}

	err = mosconfig.PublishManifest(fullproject, repo, name, manifestpath)
	if err != nil {
		return errors.Wrapf(err, "Failed writing manifest artifacts to local zot")
	}

	bootUrl := "docker://" + repo + "/" + name
	cmdline := "console=ttyS0"
	o := mosconfig.OciBoot{
		KeySet:         keyset,
		Project:        fullproject,
		BootURL:        bootUrl,
		BootStyle:      mosconfig.EFIBootModes[mosconfig.EFIAuto],
		OutFile:        outfile,
		Cdrom:          true,
		Cmdline:        cmdline,
		BootFromRemote: false,
		RepoDir:        cacheDir,
		Files:          map[string]string{},
		ZotPort:        zotPort,
	}

	return o.Build()
}

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/apex/log"
	"github.com/pkg/errors"
	"github.com/project-machine/mos/pkg/mosconfig"
	"github.com/project-machine/mos/pkg/utils"
	"github.com/urfave/cli"
)

var mkBootCmd = cli.Command{
	Name:   "mkboot",
	Usage:  "build a bootable image/livecd",
	Action: doMkBoot,
	UsageText: `url out-file
		  url: distribution URL to manifest to boot (e.g. 10.0.2.2:5000/puzzleos/hostfs:1.0.1)
		  out-file: filename to write bootable image to`,
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "boot-from-remote",
			Usage: "Configure to boot directly from network",
		},
		cli.BoolFlag{
			Name:  "cdrom",
			Usage: "create a cdrom (iso9660) rather than a disk",
		},
		cli.StringFlag{
			Name:  "cmdline",
			Usage: "cmdline: additional parameters for kernel command line",
		},
		cli.StringFlag{
			Name:  "boot",
			Usage: "boot-mode: one of 'efi-shim', 'efi-kernel', or 'efi-auto'",
			Value: mosconfig.EFIBootModes[mosconfig.EFIAuto],
		},
	},
}

func doMkBoot(ctx *cli.Context) error {
	args := ctx.Args()
	if len(args) < 3 {
		return fmt.Errorf("Need at very least 2 args: project, bootkit-source, outfile")
	}

	s := strings.SplitN(args[0], ":", 2)
	if len(s) != 2 {
		return fmt.Errorf("First argument must be keyset:project")
	}

	tmpd, err := os.MkdirTemp("", "zot")
	if err != nil {
		return errors.Wrapf(err, "Failed creating tempfile")
	}
	defer os.RemoveAll(tmpd)

	cachedir := filepath.Join(tmpd, "cache")
	utils.EnsureDir(cachedir)
	ociboot := mosconfig.OciBoot{
		KeySet:         s[0],
		Project:        s[1],
		BootURL:        args[1],
		BootStyle:      ctx.String("boot"),
		OutFile:        args[2],
		Cdrom:          ctx.Bool("cdrom"),
		Cmdline:        ctx.String("cmdline"),
		BootFromRemote: ctx.Bool("boot-from-remote"),
		RepoDir:        cachedir,
	}

	ociboot.Files = map[string]string{}
	for _, p := range ctx.StringSlice("insert") {
		toks := strings.SplitN(p, ":", 2)
		if len(toks) != 2 {
			return fmt.Errorf("--insert arg had no 'dest' (src:dest): %s", p)
		}
		ociboot.Files[toks[0]] = toks[1]
	}

	log.Debugf("Starting zot...")
	zotport, cleanup, err := mosconfig.StartZot(tmpd, cachedir)
	defer cleanup()
	if err != nil {
		return err
	}
	ociboot.ZotPort = zotport
	log.Debugf("Started zot on %d.", zotport)

	log.Debugf("Attempting mkboot with: %#v", ociboot)
	return ociboot.Build()
}

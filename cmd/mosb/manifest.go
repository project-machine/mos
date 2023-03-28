package main

import (
	"github.com/project-machine/mos/pkg/mosconfig"
	"github.com/urfave/cli"
)

var manifestCmd = cli.Command{
	Name:  "manifest",
	Usage: "build and publish a mos install manifest, and install all needed service container layers",
	Subcommands: []cli.Command{
		cli.Command{
			Name:   "publish",
			Action: doPublishManifest,
			Usage: "build and publish an install manifest",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "key",
					Usage: "path to manifest signing key to use",
					Value: "",
				},
				cli.StringFlag{
					Name:  "cert",
					Usage: "path to manifest certificate to use",
					Value: "",
				},
				cli.StringFlag{
					Name:  "repo",
					Usage: "address:port for the OCI repository to write to, e.g. 10.0.2.2:5000",
				},
				cli.StringFlag{
					Name:  "name",
					Usage: "path on OCI repository to write the processed manifest (install.json) to, e.g. puzzleos/hostfs:1.0.1",
				},
				cli.StringFlag{
					Name:  "user",
					Usage: "Username to authenticate to OCI repository",
					Value: "",
				},
				cli.StringFlag{
					Name:  "pass",
					Usage: "Password to authenticate to OCI repository.  Taken from stdin if user but no password is provided",
					Value: "",
				},
			},
		},
	},
}

func doPublishManifest(ctx *cli.Context) error {
	return mosconfig.PublishManifest(ctx)
}

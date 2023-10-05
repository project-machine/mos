package main

// Project == product

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/project-machine/mos/pkg/utils"
	"github.com/urfave/cli"
)

var projectCmd = cli.Command{
	Name:  "project",
	Usage: "Generate a uuid and keypair",
	Subcommands: []cli.Command{
		cli.Command{
			Name:      "list",
			Action:    doListProjects,
			Usage:     "list projects",
			ArgsUsage: "<keyset-name>",
		},
		cli.Command{
			Name:      "add",
			Action:    doAddProject,
			Usage:     "add a new project",
			ArgsUsage: "<keyset-name> <project-name>",
		},
	},
}

func doAddProject(ctx *cli.Context) error {
	args := ctx.Args()
	if len(args) != 2 {
		return errors.New("Projects belong to a keyset. Specify keyset name to list the projects in a keyset.")
	}

	keysetName := args[0]
	projName := args[1]

	trustDir, err := utils.GetMosKeyPath()
	if err != nil {
		return err
	}

	keysetPath := filepath.Join(trustDir, keysetName)
	projPath := filepath.Join(keysetPath, "manifest", projName)
	if utils.PathExists(projPath) {
		return fmt.Errorf("Project %s already exists", projName)
	}

	if err = os.Mkdir(projPath, 0750); err != nil {
		return errors.Wrapf(err, "Failed creating project directory %q", projPath)
	}

	// Create new manifest credentials
	err = generateNewUUIDCreds(keysetName, projPath)
	if err != nil {
		os.RemoveAll(projPath)
		return errors.Wrapf(err, "Failed creating new project")
	}

	if err := utils.EnsureDir(filepath.Join(projPath, "sudi")); err != nil {
		os.RemoveAll(projPath)
		return errors.Wrapf(err, "Failed creating sudi directory for new project")
	}

	fmt.Printf("New credentials saved in %s directory\n", projPath)
	return nil
}

func doListProjects(ctx *cli.Context) error {
	args := ctx.Args()
	if len(args) == 0 {
		return errors.New("Projects belong to a keyset. Specify keyset name to list the projects in a keyset.")
	}

	keysetName := args[0]
	trustDir, err := utils.GetMosKeyPath()
	if err != nil {
		return err
	}
	keysetPath := filepath.Join(trustDir, keysetName)
	if !utils.PathExists(keysetPath) {
		return fmt.Errorf("Keyset not found: %s", keysetName)
	}

	keysetPath = filepath.Join(keysetPath, "manifest")
	if !utils.PathExists(keysetPath) {
		fmt.Printf("No projects found")
		return nil
	}

	dirs, err := os.ReadDir(keysetPath)
	if err != nil {
		return fmt.Errorf("Failed reading keys directory %q: %w", trustDir, err)
	}

	for _, keyname := range dirs {
		fmt.Printf("%s\n", keyname.Name())
	}

	return nil
}

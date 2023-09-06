package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"

	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/umoci/oci/casext"
	"github.com/opencontainers/umoci/oci/layer"
	"github.com/plus3it/gorecurcopy"
	"golang.org/x/sys/unix"
	"stackerbuild.io/stacker/pkg/atomfs"
	"stackerbuild.io/stacker/pkg/container/idmap"
	stackeroci "stackerbuild.io/stacker/pkg/oci"
)

func PathExists(d string) bool {
	_, err := os.Stat(d)
	if err != nil && os.IsNotExist(err) {
		return false
	}
	return true
}

func RunCommand(args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s: %s", strings.Join(args, " "), err, string(output))
	}
	return nil
}

// Which - like unix which(1) - search PATH environment variable for name.
func Which(name string) string {
	return WhichSearch(name, strings.Split(os.Getenv("PATH"), ":"))
}

// WhichSearch - search through paths for executable with name.
func WhichSearch(name string, paths []string) string {
	var search []string

	if strings.ContainsRune(name, os.PathSeparator) {
		if path.IsAbs(name) {
			search = []string{name}
		} else {
			search = []string{"./" + name}
		}
	} else {
		search = []string{}
		for _, p := range paths {
			search = append(search, path.Join(p, name))
		}
	}

	for _, fPath := range search {
		if err := unix.Access(fPath, unix.X_OK); err != nil {
			continue
		}
		s, err := os.Stat(fPath)
		if err != nil {
			continue
		}
		if !s.Mode().IsRegular() {
			continue
		}
		return fPath
	}

	return ""
}

func GetRootlessMapOptions() (layer.MapOptions, error) {
	opts := layer.MapOptions{Rootless: true}
	idmapSet, err := idmap.ResolveCurrentIdmapSet()
	if err != nil {
		return opts, err
	}

	if idmapSet == nil {
		return opts, fmt.Errorf("no uids mapped for current user")
	}

	for _, idm := range idmapSet.Idmap {
		if err := idm.Usable(); err != nil {
			return opts, fmt.Errorf("idmap unusable: %s", err)
		}

		if idm.Isuid {
			opts.UIDMappings = append(opts.UIDMappings, rspec.LinuxIDMapping{
				ContainerID: uint32(idm.Nsid),
				HostID:      uint32(idm.Hostid),
				Size:        uint32(idm.Maprange),
			})
		}

		if idm.Isgid {
			opts.GIDMappings = append(opts.GIDMappings, rspec.LinuxIDMapping{
				ContainerID: uint32(idm.Nsid),
				HostID:      uint32(idm.Hostid),
				Size:        uint32(idm.Maprange),
			})
		}
	}

	return opts, nil
}

func unpackSquashLayer(ociDir string, oci casext.Engine, tag string, dest string, rootless bool) error {
	rootfsDir := path.Join(dest, "rootfs")
	manifest, err := stackeroci.LookupManifest(oci, tag)
	if err != nil {
		return err
	}

	if !rootless {
		if err := unpackSquashLayersViaAtomfs(ociDir, tag, rootfsDir); err != nil {
			// if error was not due to mount permissions or squashfs support in kernel
			// then probably should just return here...
			return fmt.Errorf("failed to unpack %s in ociDir %s via atomfs: %s",
				tag, ociDir, err)
		} else {
			return nil
		}
	}

	for _, layer := range manifest.Layers {
		squashFile := path.Join(ociDir, "blobs/sha256", layer.Digest.Encoded())
		if err := extractSingleSquash(squashFile, rootfsDir, rootless); err != nil {
			return err
		}
	}

	return nil
}

func unpackSquashLayersViaAtomfs(ociDir string, tag string, dest string) error {
	mounted := false
	tmpdir, err := ioutil.TempDir("", "unsquashLayers")
	if err != nil {
		return err
	}
	mdPath := path.Join(tmpdir, "meta-data")
	mountpoint := path.Join(tmpdir, "mount")

	cleanup := func(err error) error {
		if mounted {
			if cleanupErr := atomfs.Umount(mountpoint); cleanupErr != nil {
				if err != nil {
					return fmt.Errorf("Umount failed (%v) after error: %v", cleanupErr, err)
				}
				return cleanupErr
			}
		}

		if cleanupErr := os.RemoveAll(tmpdir); cleanupErr != nil {
			if err != nil {
				return fmt.Errorf("Tmpdir cleanup failed (%v) after error: %v", cleanupErr, err)
			}
			return cleanupErr
		}

		return err
	}

	if err := os.Mkdir(mdPath, 0755); err != nil {
		return cleanup(err)
	}

	if err := os.Mkdir(mountpoint, 0755); err != nil {
		return cleanup(err)
	}

	opts := atomfs.MountOCIOpts{
		OCIDir:       ociDir,
		MetadataPath: mdPath,
		Tag:          tag,
		Target:       mountpoint,
	}

	mol, err := atomfs.BuildMoleculeFromOCI(opts)
	if err != nil {
		return cleanup(err)
	}

	if err = mol.Mount(mountpoint); err != nil {
		return err
	}

	mounted = true

	return cleanup(gorecurcopy.CopyDirectory(mountpoint, dest))

}

func extractSingleSquash(squashFile string, extractDir string, rootless bool) error {
	err := os.MkdirAll(extractDir, 0755)
	if err != nil {
		return err
	}

	var cmd []string
	if Which("squashtool") != "" {
		cmd = []string{"squashtool", "extract", "--whiteouts", "--perms"}
		if !rootless {
			cmd = append(cmd, "--devs", "--sockets", "--owners")
		}
		cmd = append(cmd, squashFile, extractDir)
	} else {
		cmd = []string{"unsquashfs", "-f", "-d", extractDir, squashFile}
	}
	return RunCommand(cmd...)
}

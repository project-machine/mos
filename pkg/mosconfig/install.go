package mosconfig

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/apex/log"
	"stackerbuild.io/stacker/lib"
)

func InitializeMos(cf *InstallFile, storeDir, configDir, baseDir string) error {
	mos, err := NewMos(configDir, storeDir)
	if err != nil {
		return err
	}
	defer mos.Close()

	for _, target := range cf.Targets {
		err = mos.ExtractTarget(baseDir, &target)
		if err != nil {
			return err
		}
	}

	return nil
}

// baseDir is "", in which case we fetch zot layers over the
// network, or
// baseDir has $baseDir/oci/ under which we find the layers.
// or baseDir has $baseDir/zot/ under which we find the layers.
// We copy the layers into $storeDir in zot format.
func (mos *Mos) ExtractTarget(baseDir string, target *Target) error {
	if baseDir == "" {
		return fmt.Errorf("remote zot copy not yet implemented")
	}
	ociDir := filepath.Join(baseDir, "oci")
	zotDir := filepath.Join(baseDir, "zot")
	if !PathExists(ociDir) {
		if PathExists(zotDir) {
			return fmt.Errorf("local zot layout not yet supported")
		}
		return fmt.Errorf("no oci or zot storage found under %s", baseDir)
	}
	src := fmt.Sprintf("oci:%s:%s", filepath.Join(baseDir, "oci"), target.Name)
	dest := fmt.Sprintf("oci:%s/%s:%s", mos.opts.StorageCache, target.Name, target.Version())

	log.Infof("copying %s from '%s' into zot as '%s'", target.Name, src, dest)

	copyOpts := lib.ImageCopyOpts{Src: src, Dest: dest, Progress: os.Stdout}
	if err := lib.ImageCopy(copyOpts); err != nil {
		return fmt.Errorf("failed copying layer %v: %w", target, err)
	}

	return nil
}

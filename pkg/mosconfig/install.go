package mosconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/apex/log"
	"github.com/pkg/errors"
	"stackerbuild.io/stacker/pkg/lib"
)

func InitializeMos(cf *InstallFile, storeDir, configDir, baseDir string) error {
	// We must have $basedir/install.yml and $basedir/cert.pem
	mPath := filepath.Join(baseDir, "install.yaml")
	cPath := filepath.Join(baseDir, "manifestCert.pem")
	sPath := filepath.Join(baseDir, "install.yaml.signed")
	caPath := filepath.Join(baseDir, "manifestCA.pem")
	if !PathExists(mPath) || !PathExists(cPath) || !PathExists(sPath) || !PathExists(caPath) {
		return fmt.Errorf("Install manifest or certificate missing")
	}
	mos, err := NewMos(configDir, storeDir)
	if err != nil {
		return fmt.Errorf("Error opening manifest: %w", err)
	}
	defer mos.Close()

	for _, target := range cf.Targets {
		err = mos.ExtractTarget(baseDir, &target)
		if err != nil {
			return err
		}
	}

	if cf.UpdateType == PartialUpdate {
		return fmt.Errorf("Cannot install with a partial manifest")
	}

	// Finally set up our manifest store
	err = initManifest(cf, mPath, cPath, caPath, configDir)
	if err != nil {
		return fmt.Errorf("Error initializing system manifest: %w", err)
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
	zotDir := filepath.Join(baseDir, "zot")
	ociDir := filepath.Join(baseDir, "oci")
	var err error
	switch {
	case PathExists(ociDir):
		err = mos.copyLocalOci(ociDir, target)
	case PathExists(zotDir):
		err = mos.copyLocalZot(zotDir, target)
	default:
		err = fmt.Errorf("no oci or zot storage found under %s", baseDir)
	}

	return errors.Wrapf(err, "Error extracting target %#v", target)
}

// return the fullname and version from a zot url.  For instance,
// fullnameFromUrl("docker://zothub.io/c3/base:latest") returns
// "c3/base", "latest", nil
func fullnameFromUrl(url string) (string, string, error) {
	prefix := "docker://"
	prefixLen := len(prefix)
	if !strings.HasPrefix(url, prefix) {
		return "", "", fmt.Errorf("Bad zot URL: bad prefix")
	}
	url = url[prefixLen:]
	addrsplit := strings.SplitN(url, "/", 2)
	if len(addrsplit) < 2 {
		return "", "", fmt.Errorf("Bad zot URL: no address")
	}
	tagname := addrsplit[1]
	idx := strings.LastIndex(tagname, ":")
	if idx == -1 {
		return "", "", fmt.Errorf("Bad zot URL: no tag")
	}
	name := tagname[:idx]
	version := tagname[idx+1:]
	if len(name) < 1 || len(version) < 1 {
		return "", "", fmt.Errorf("Bad zot URL: short name or tag")
	}
	return name, version, nil
}

func (mos *Mos) copyLocalZot(zotDir string, target *Target) error {
	layerDir := filepath.Join(zotDir, target.ZotPath)
	src := fmt.Sprintf("oci:%s:%s", layerDir, target.Version)
	tpath := filepath.Join(mos.opts.StorageCache, target.ZotPath)
	if err := EnsureDir(tpath); err != nil {
		return fmt.Errorf("Failed creating local zot directory %q: %w", tpath, err)
	}
	dest := fmt.Sprintf("oci:%s:%s", tpath, target.Version)

	log.Infof("copying %q:%s from local zot ('%s') into zot as '%s'", target.ZotPath, target.Version, src, dest)

	copyOpts := lib.ImageCopyOpts{Src: src, Dest: dest, Progress: os.Stdout}
	if err := lib.ImageCopy(copyOpts); err != nil {
		return fmt.Errorf("failed copying layer %v: %w", target, err)
	}

	return nil
}

func (mos *Mos) copyLocalOci(ociDir string, target *Target) error {
	src := fmt.Sprintf("oci:%s:%s", ociDir, target.ServiceName)
	tpath := filepath.Join(mos.opts.StorageCache, target.ZotPath)
	err := EnsureDir(tpath)
	if err != nil {
		return fmt.Errorf("Failed creating local zot directory %q: %w", tpath, err)
	}
	dest := fmt.Sprintf("oci:%s:%s", tpath, target.Version)

	log.Infof("copying %s from local oci ('%s') into zot as '%s'", target.ServiceName, src, dest)

	copyOpts := lib.ImageCopyOpts{Src: src, Dest: dest, Progress: os.Stdout}
	if err := lib.ImageCopy(copyOpts); err != nil {
		return fmt.Errorf("failed copying layer %v: %w", target, err)
	}

	return nil
}

package mosconfig

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	satomfs "stackerbuild.io/stacker/pkg/atomfs"
)

func MountOCILayer(ocidir, name, dest string) (func(), error) {
	metadir := filepath.Join(dest, "meta") // directory for atomfs metadata
	if err := EnsureDir(metadir); err != nil {
		return func() {}, errors.Errorf("Failed creating metadata directory")
	}

	cleanup := func() { os.RemoveAll(metadir) }
	opts := satomfs.MountOCIOpts{
		OCIDir:       ocidir,
		MetadataPath: metadir,
		Tag:          name,
		Target:       dest,
	}
	mol, err := satomfs.BuildMoleculeFromOCI(opts)
	if err != nil {
		return cleanup, err
	}

	err = mol.Mount(dest)
	if err != nil {
		return cleanup, errors.Wrapf(err, "Failed mounting atomfs")
	}
	cleanup = func() {
		satomfs.Umount(dest)
		os.RemoveAll(metadir)
	}

	return cleanup, nil
}

func MountSOCI(ocidir, metalayer, capath, mountpoint string) error {
	tmpd, err := os.MkdirTemp("", "extract")
	if err != nil {
		return errors.Wrapf(err, "Failed creating tempdir")
	}

	cleanup, err := MountOCILayer(ocidir, metalayer, tmpd)
	if err != nil {
		return errors.Wrapf(err, "Failed unpacking SOCI metalayer layer")
	}
	defer cleanup()

	mPath := filepath.Join(tmpd, "manifest.yaml")
	sPath := mPath + ".signed"
	cPath := filepath.Join(tmpd, "manifestCert.pem")
	if !PathExists(mPath) || !PathExists(sPath) || !PathExists(cPath) {
		return errors.Errorf("bad SOCI layer")
	}

	// Set up a temporary storage
	opts := DefaultMosOptions()
	opts.CaPath = capath
	opts.StorageCache = ocidir

	s, err := NewStorage(opts)
	if err != nil {
		return err
	}
	manifest, err := ReadVerifyManifest(mPath, cPath, capath, "", s)
	if err != nil {
		fmt.Printf("Failed verifying %q using %q and %q\n", mPath, cPath, capath)
		return errors.Wrapf(err, "Verification of manifest on metalayer failed")
	}

	if len(manifest.Targets) != 1 {
		return errors.Errorf("manifest.yaml must have precisely one target")
	}
	t := manifest.Targets[0]

	_, err = MountOCILayer(ocidir, t.ServiceName, mountpoint)
	if err != nil {
		return errors.Wrapf(err, "Failed mounting %s", t.ServiceName)
	}

	return nil
}

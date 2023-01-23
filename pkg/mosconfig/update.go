package mosconfig

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

func (mos *Mos) Update(filename string) error {
	filename, err := filepath.Abs(filename)
	if err != nil {
		return fmt.Errorf("Failed to make absolute pathname for install file: %w", err)
	}
	baseDir := filepath.Dir(filename)
	cPath := filepath.Join(baseDir, "manifestCert.pem")
	sPath := filepath.Join(baseDir, "install.yaml.signed")
	if !PathExists(filename) || !PathExists(cPath) || !PathExists(sPath) {
		return fmt.Errorf("Install manifest or certificate missing")
	}

	manifest, err := mos.CurrentManifest()
	if err != nil {
		return err
	}

	shaSum, err := ShaSum(filename)
	if err != nil {
		return fmt.Errorf("Failed calculating shasum: %w", err)
	}

	newIF, err := ReadVerifyManifest(filename, cPath, mos.opts.CaPath, baseDir, mos.storage)
	if err != nil {
		return fmt.Errorf("Failed verifying signature on %s: %w", filename, err)
	}

	// The shasum-named install.yaml which we'll place in
	// /config/manifest.git
	mFile := fmt.Sprintf("%s.yaml", shaSum)
	sFile := fmt.Sprintf("%s.yaml.signed", shaSum)
	cFile := fmt.Sprintf("%s.pem", shaSum)

	newtargets := SysTargets{}

	for _, t := range newIF.Targets {
		newT := SysTarget{
			Name:   t.ServiceName,
			Source: mFile,
			raw:    &t,
		}
		newtargets = append(newtargets, newT)
		if err := mos.storage.ImportTarget(baseDir, &t); err != nil {
			return fmt.Errorf("Failed copying %s: %w", newT.Name, err)
		}
	}

	sysmanifest, err := mergeUpdateTargets(manifest, newtargets, newIF.UpdateType)
	if err != nil {
		return err
	}

	tmpdir, err := os.MkdirTemp(filepath.Join(mos.opts.RootDir, "/root"), "newmanifest")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpdir)

	dest := filepath.Join(tmpdir, mFile)
	if err = CopyFileBits(filename, dest); err != nil {
		return fmt.Errorf("Failed copying %q to %q: %w", filename, dest, err)
	}

	dest = filepath.Join(tmpdir, sFile)
	if err = CopyFileBits(sPath, dest); err != nil {
		return fmt.Errorf("Failed copying %q to %q: %w", sPath, dest, err)
	}

	dest = filepath.Join(tmpdir, cFile)
	if err = CopyFileBits(cPath, dest); err != nil {
		return fmt.Errorf("Failed copying %q to %q: %w", cPath, dest, err)
	}

	bytes, err := yaml.Marshal(&sysmanifest)
	if err != nil {
		return fmt.Errorf("Failed marshalling the system manifest")
	}

	dest = filepath.Join(tmpdir, "manifest.yaml")
	if err = os.WriteFile(dest, bytes, 0640); err != nil {
		return fmt.Errorf("Failed writing system manifest: %w", err)
	}

	if err = mos.UpdateManifest(manifest, &sysmanifest, tmpdir); err != nil {
		return err
	}

	return nil
}

// Any target in old which is also listed in updated, gets
// switched for the one in updated.  Any target in updated
// which is not in old gets appended.
func mergeUpdateTargets(old *SysManifest, updated SysTargets, updateType UpdateType) (SysManifest, error) {
	newtargets := SysTargets{}
	if updateType == PartialUpdate {
		for _, t := range old.SysTargets {
			if _, contained := updated.Contains(t); !contained {
				newtargets = append(newtargets, t)
			}
		}
	}

	for _, t := range updated {
		newtargets = append(newtargets, t)
	}

	uidmaps := []IdmapSet{}
	for _, t := range newtargets {
		uidmaps = addUIDMap(old.UidMaps, uidmaps, *t.raw)
	}

	return SysManifest{
		UidMaps: uidmaps,
		SysTargets: newtargets,
	}, nil
}

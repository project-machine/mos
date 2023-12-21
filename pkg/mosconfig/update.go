package mosconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/project-machine/mos/pkg/utils"
)

func (mos *Mos) Update(url string) error {
	var is InstallSource
	defer is.Cleanup()

	err := is.FetchFromZot(url)
	if err != nil {
		return err
	}

	manifest, err := mos.CurrentManifest()
	if err != nil {
		return err
	}

	// TODO - we will drop the git manifest altogether.
	shaSum, err := utils.ShaSum(is.FilePath)
	if err != nil {
		return fmt.Errorf("Failed calculating shasum: %w", err)
	}

	newIF, err := ReadVerifyInstallManifest(is, mos.opts.CaPath, mos.storage)
	if err != nil {
		return errors.Wrapf(err, "Failed verifying signature on %s", is.FilePath)
	}

	// The shasum-named install.json which we'll place in
	// /config/manifest.git
	mFile := fmt.Sprintf("%s.json", shaSum)
	sFile := fmt.Sprintf("%s.json.signed", shaSum)
	cFile := fmt.Sprintf("%s.pem", shaSum)

	newtargets := SysTargets{}

	for _, t := range newIF.Targets {
		newT := SysTarget{
			Name:   t.ServiceName,
			Source: mFile,
			raw:    &t,
		}
		newtargets = append(newtargets, newT)
		src := fmt.Sprintf("docker://%s/mos:%s", is.ocirepo.addr, dropHashAlg(t.Digest))
		if err := mos.storage.ImportTarget(src, &t); err != nil {
			return fmt.Errorf("Failed copying %s: %w", newT.Name, err)
		}
	}

	sysmanifest, err := mergeUpdateTargets(manifest, newtargets, newIF.Storage, newIF.UpdateType)
	if err != nil {
		return err
	}

	tmpdir, err := os.MkdirTemp("", "newmanifest")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpdir)

	dest := filepath.Join(tmpdir, mFile)
	if err = utils.CopyFileBits(is.FilePath, dest); err != nil {
		return fmt.Errorf("Failed copying %q to %q: %w", is.FilePath, dest, err)
	}

	dest = filepath.Join(tmpdir, sFile)
	if err = utils.CopyFileBits(is.SignPath, dest); err != nil {
		return fmt.Errorf("Failed copying %q to %q: %w", is.SignPath, dest, err)
	}

	dest = filepath.Join(tmpdir, cFile)
	if err = utils.CopyFileBits(is.CertPath, dest); err != nil {
		return fmt.Errorf("Failed copying %q to %q: %w", is.CertPath, dest, err)
	}

	bytes, err := json.Marshal(&sysmanifest)
	if err != nil {
		return fmt.Errorf("Failed marshalling the system manifest")
	}

	dest = filepath.Join(tmpdir, "manifest.json")
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
func mergeUpdateTargets(old *SysManifest, updated SysTargets, s StorageList, updateType UpdateType) (SysManifest, error) {
	newtargets := SysTargets{}
	newstorage := StorageList{}
	if updateType == PartialUpdate {
		for _, t := range old.SysTargets {
			if !updated.Contains(t) {
				newtargets = append(newtargets, t)
			}
		}
		for _, n := range old.Storage {
			if !s.Contains(n) {
				newstorage = append(newstorage, n)
			}
		}
	}

	for _, t := range updated {
		newtargets = append(newtargets, t)
	}

	for _, n := range s {
		newstorage = append(newstorage, n)
	}

	uidmaps := []IdmapSet{}
	for _, t := range newtargets {
		uidmaps = addUIDMap(old.UidMaps, uidmaps, t.raw.NSGroup)
	}
	for _, n := range newstorage {
		uidmaps = addUIDMap(old.UidMaps, uidmaps, n.NSGroup)
	}

	return SysManifest{
		UidMaps:    uidmaps,
		SysTargets: newtargets,
		Storage:    newstorage,
	}, nil
}

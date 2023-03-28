package mosconfig

import (
	"path/filepath"

	"github.com/pkg/errors"
)

// getTargetAt: Get the target called @target from the current host manifest,
// or, if @url is not "", then get it from the remote manifest.  The remote
// manifest must be signed by a certificate validated by our CA.
func (mos *Mos) getTargetAt(target, inUrl string) (*Target, error) {
	if inUrl == "" {
		sm, err := mos.CurrentManifest()
		if err != nil {
			return &Target{}, err
		}
		t, err := sm.GetTarget(target)
		if err != nil {
			return &Target{}, errors.Wrapf(err, "Failed to find target to mount: %q", target)
		}
		return t.raw, nil
	}

	m, err := mos.remoteManifest(inUrl)
	if err != nil {
		return &Target{}, err
	}

	t, err := m.GetTarget(target)
	if err != nil {
		return &Target{}, err
	}
	return t, nil
}

func (mos *Mos) Mount(target, dest, url string, ro bool) error {
	if target == "" {
		target = "hostfs"
	}

	t, err := mos.getTargetAt(target, url)
	if err != nil {
		mstr := ""
		if url != "" {
			mstr = " at " + url
		}
		return errors.Wrapf(err, "Failed finding %s%s", target, mstr)
	}

	if dest == "" {
		dest = filepath.Join(mos.opts.RootDir, "/mnt/mos", target)
	}

	if err := EnsureDir(dest); err != nil {
		return errors.Wrapf(err, "Failed creating mountpoint")
	}

	if ro {
		_, err = mos.storage.Mount(t, dest)
	} else {
		_, err = mos.storage.MountWriteable(t, dest)
	}

	if err != nil {
		return errors.Wrapf(err, "Failed mounting %q onto %q", target, dest)
	}

	return nil
}

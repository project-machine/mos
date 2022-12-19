package mosconfig

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/apex/log"
	"golang.org/x/sys/unix"
	"stackerbuild.io/stacker/atomfs"
)

type StorageType string

const (
	AtomfsStorageType   StorageType = "atomfs"
	PuzzlefsStorageType StorageType = "puzzlefs"
)

type Storage interface {
	Type() StorageType

	Mount(t *Target, mountpoint string) (func(), error)
	MountWriteable(t *Target, mountpoint string) (func(), error)
}

func NewStorage(opts MosOptions) (Storage, error) {
	var s Storage
	var e error
	switch opts.StorageType {
	case AtomfsStorageType:
		s, e = NewAtomfsStorage(opts.StorageCache, opts.ScratchWrites)
	case PuzzlefsStorageType:
		return nil, fmt.Errorf("Not yet implemented")
	default:
		return nil, fmt.Errorf("Unknown storage type requested")
	}

	return s, e
}

type AtomfsStorage struct {
	zotPath      string
	scratchPath  string
}

func NewAtomfsStorage(zotPath, scratchPath string) (*AtomfsStorage, error) {
	return &AtomfsStorage{
		zotPath: zotPath,
		scratchPath: scratchPath,
	}, nil
}

func (a *AtomfsStorage) Type() StorageType {
	return AtomfsStorageType
}

// The metadata path which we pass to 'stacker/atomfs' is the directory 'atomfs'
// under *our* scratchdir.
func (a *AtomfsStorage) metadataPath() string {
	return filepath.Join(a.scratchPath, "atomfs")
}

func (a *AtomfsStorage) Mount(t *Target, mountpoint string) (func(), error) {
	opts := atomfs.MountOCIOpts{
		OCIDir:       filepath.Join(a.zotPath, t.Fullname),
		MetadataPath: a.metadataPath(),
		Tag:          t.Version,
		Target:       mountpoint,
	}

	mol, err := atomfs.BuildMoleculeFromOCI(opts)
	if err != nil {
		return func() {}, err
	}

	cleanup := func() {
		err := atomfs.Umount(mountpoint)
		if err != nil {
			log.Warnf("unmounting %s failed: %s", mountpoint, err)
		}
	}
	return cleanup, mol.Mount(mountpoint)
}

func (a *AtomfsStorage) MountWriteable(t *Target, mountpoint string) (func(), error) {
	ropath, err := os.MkdirTemp(a.scratchPath, fmt.Sprintf("%s-scratch-readonly-", t.Name))
	if err != nil {
		return func() {}, fmt.Errorf("Failed creating readonly mountpoint: %w", err)
	}

	roCleanup, err := a.Mount(t, ropath)
	if err != nil {
		os.Remove(ropath)
		return func() {}, fmt.Errorf("Failed creating readonly mount for %#v: %w", t, err)
	}

	workdir, err := os.MkdirTemp(a.scratchPath, fmt.Sprintf("%s-scratch-workdir-", t.Name))
	if err != nil {
		roCleanup()
		os.Remove(ropath)
		return func() {}, fmt.Errorf("Failed creating workdir: %w", err)
	}

	upperdir, err := os.MkdirTemp(a.scratchPath, fmt.Sprintf("%s-scratch-upperdir-", t.Name))
	if err != nil {
		roCleanup()
		os.Remove(ropath)
		os.RemoveAll(workdir)
		return func() {}, fmt.Errorf("Failed creating upperdir: %w", err)
	}

	overlayArgs := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", ropath, upperdir, workdir)
	err = unix.Mount("overlayfs", mountpoint, "overlay", 0, overlayArgs)
	if err != nil {
		roCleanup()
		os.RemoveAll(workdir)
		os.RemoveAll(upperdir)
		os.Remove(ropath)
		return nil, err
		return func() {}, fmt.Errorf("Failed mounting writeable overlay: %w", err)
	}
	cleanup := func() {
		unix.Unmount(mountpoint, 0)
		roCleanup()
		os.RemoveAll(workdir)
		os.RemoveAll(upperdir)
		os.Remove(ropath)
	}

	return cleanup, nil
}

package mosconfig

import (
	"fmt"
	"path/filepath"

	"github.com/apex/log"
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

func (a *AtomfsStorage) Mount(t *Target, mountpoint string) (func(), error) {
	opts := atomfs.MountOCIOpts{
		OCIDir:       filepath.Join(a.zotPath, t.Fullname),
		MetadataPath: a.scratchPath,
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
	return func() { }, fmt.Errorf("Not yet implemented")
}

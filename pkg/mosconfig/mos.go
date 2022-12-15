package mosconfig

import (
	"os"
)

type MosOptions struct {
	// Storage type - atomfs for now
	StorageType StorageType

	// Where install manifest is found
	ConfigDir     string

	// Directory where atomfs/puzzlefs cache is rooted
	StorageCache  string

	// whether this will be a session which writes to some mos state
	LayersReadOnly bool

	// whether this will be a session which writes to the system manifest
	ManifestReadOnly bool

	// During initial install, we can't read the provisioned host certs
	NoHostCerts bool
}

func DefaultMosOptions() *MosOptions {
	return &MosOptions{
		ConfigDir:   "/config",
		StorageType: AtomfsStorageType,
		StorageCache: "/scratch-writes",
	}
}

type Mos struct {
	//storage   Storage
	//manifest  Manifest
	//bootmgr   Bootmgr
	opts        MosOptions
	lockfile    *os.File
}

func NewMos(configDir, storeDir string) (*Mos, error) {
	opts := MosOptions{
		StorageType: AtomfsStorageType,
		ConfigDir: configDir,
		StorageCache: storeDir,
		LayersReadOnly: false,
		ManifestReadOnly: false,
		NoHostCerts: true,
	}

	mos := &Mos{
		opts: opts,
		lockfile: nil,
	}
	err := mos.acquireLock()
	if err != nil {
		return mos, err
	}
	return mos, nil
}

func OpenMos(opts MosOptions) (*Mos, error) {
	mos := &Mos{opts: opts}

	err := mos.acquireLock()
	if err != nil {
		return nil, err
	}
	return mos, nil
}

func (mos *Mos) Close() {
	if mos.lockfile != nil {
		mos.lockfile.Close()
		mos.lockfile = nil
	}
}

package mosconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/apex/log"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

type MosOptions struct {
	// Storage type - atomfs for now
	StorageType StorageType

	// The host root directory.  If you specify this (to anything other
	// than "/"), then the ConfigDir, StorageCache, and ScratchWrites
	// will be set relative to it.
	RootDir       string

	// Where install manifest is found
	ConfigDir     string

	// Directory where atomfs/puzzlefs cache is rooted
	StorageCache  string

	// Directory where atomfs keeps its working storage
	// e.g. upperdirs and temporary mounts
	ScratchWrites  string

	// whether this will be a session which writes to some mos state
	LayersReadOnly bool

	// whether this will be a session which writes to the system manifest
	ManifestReadOnly bool

	// During initial install, we can't read the provisioned host certs
	NoHostCerts bool

	// OTOH if we want to fetch the manifest CA from a custom path:
	CaPath string
}

func DefaultMosOptions() MosOptions {
	return MosOptions{
		StorageType:      AtomfsStorageType,
		ConfigDir:        "",
		StorageCache:     "",
		ScratchWrites:    "",
		RootDir:          "/",
		LayersReadOnly:   true,
		ManifestReadOnly: true,
		NoHostCerts:      false,
		CaPath:           "/factory/secure/manifestCA.pem",
	}
}

type Mos struct {
	storage     Storage
	//bootmgr   Bootmgr

	CaPath      string
	opts        MosOptions
	lockfile    *os.File
}

func NewMos(configDir, storeDir string) (*Mos, error) {
	opts := MosOptions{
		StorageType: AtomfsStorageType,
		ConfigDir: configDir,
		StorageCache: storeDir,
		RootDir: "/",
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

func setDirOpts(opts MosOptions) MosOptions {
	if opts.RootDir == "" {
		opts.RootDir = "/"
	}
	if opts.ConfigDir == "" {
		opts.ConfigDir = filepath.Join(opts.RootDir, "config")
	}
	if opts.StorageCache == "" {
		opts.StorageCache = filepath.Join(opts.RootDir, "atomfs-store")
	}
	if opts.ScratchWrites == "" {
		opts.ScratchWrites = filepath.Join(opts.RootDir, "scratch-writes")
	}
	return opts
}

func OpenMos(opts MosOptions) (*Mos, error) {
	opts = setDirOpts(opts)

	s, err := NewStorage(opts)
	if err != nil {
		return nil, fmt.Errorf("Error initializing storage")
	}

	mos := &Mos{
		opts: opts,
		storage: s,
	}

	err = mos.acquireLock()
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

func (mos *Mos) Storage() Storage {
	return mos.storage
}

// Give the current information for the target named @name, for instance
// 'hostfs' or 'zot.  Returns a *Target containing the full target
// information from the manifest
func (mos *Mos) Current(name string) (*Target, error) {
	systargets, err := mos.CurrentManifest()
	if err != nil {
		return nil, errors.Wrapf(err, "Failed opening manifest")
	}

	for _, t := range systargets {
		if t.Name == name {
			return t.raw, nil
		}
	}

	return nil, errors.Errorf("Target %s not found", name)
}

// Activate a target (service):
// If it is not yet running then start it.
// If it is already running, but is not at the newest version (i.e. after an
// upgrade), then restart it. (Not yet implemented)
func (mos *Mos) Activate(name string) error {
	unitName := fmt.Sprintf("%s.service", name)
	t, err := mos.Current(name)
	if err != nil {
		return err
	}

	if t.ServiceType == HostfsService {
		return fmt.Errorf("Reboot not yet supported, do it yourself")
	}

	v, err := mos.RunningVersion(t)
	if err != nil {
		return err
	}

	// TODO we'll need to find the version from the hash?
	log.Debugf("running version is \"%q\" wanted version is %q", v, t.Version)
	if v == t.Version {
		// latest version already running
		return nil
	}

	if v != "" {
		log.Infof("Stopping target %q", t.Name)
		err = mos.StopTarget(t)
		if err != nil {
			return fmt.Errorf("Failed stopping service %s for update: %w", name, err)
		}
		log.Infof("Stopped target %q", t.Name)
	}

	err = mos.SetupTargetRuntime(t)
	if err != nil {
		return err
	}

	if t.ServiceType != ContainerService {
		return nil
	}

	return RunCommand("systemctl", "start", "--no-block", unitName)
}

func (mos *Mos) SetupTargetRuntime(t *Target) error {
	log.Debugf("Setting up target %s", t.Name)
	err := mos.storage.SetupTarget(t)
	if err != nil {
		return fmt.Errorf("Failed setting up storage for %s:%s: %w", t.Name, t.Version, err)
	}

	switch t.ServiceType {
	case FsService:
		src, err := mos.storage.TargetMountdir(t)
		if err != nil {
			return err
		}
		dest := filepath.Join(mos.opts.RootDir, "/mnt/atom", t.Name)
		if err := EnsureDir(dest); err != nil {
			return fmt.Errorf("Unable to create directory %s: %w", dest, err)
		}
		if err = unix.Mount(src, dest, "", unix.MS_BIND, ""); err != nil {
			return err
		}
	case HostfsService:
		return nil
	case ContainerService:
		return fmt.Errorf("Container service type not yet implemented")
	default:
		return fmt.Errorf("Unhandled service type %s", t.ServiceType)
	}

	return nil
}

// Return the layer hash for a running service.
// We do this by looking for the mounted fs and using the hash to look
// back through the manifest and find the current version.
// Return "", nil if the service is not running.
func (mos *Mos) RunningVersion(t *Target) (string, error) {
	hash, err := mos.storage.MountedByHash(t)
	if err != nil {
		return "", err
	}

	return hash, nil
}

func (mos *Mos) StopTarget(t *Target) error {
	unitName := fmt.Sprintf("%s.service", t.Name)
	switch t.ServiceType {
	case ContainerService:
		out, rc := RunCommandWithRc("systemctl", "stop", unitName)
		outs := string(out)
		if rc != 0 && !strings.HasSuffix(outs, "not loaded.\n") {
			return fmt.Errorf("Failed to stop service %s: %s", t.Name, outs)
		}
	case HostfsService:
		return fmt.Errorf("Stopping hostfs is not yet supported.  Please poweroff")
	}

	err := mos.storage.TearDownTarget(t.Name)
	if err !=  nil {
		return fmt.Errorf("Failed shutting down storage for %s: %w", t.Name, err)
	}

	return nil
}

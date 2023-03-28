package mosconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	RootDir string

	// Where install manifest is found
	ConfigDir string

	// Directory where atomfs/puzzlefs cache is rooted
	StorageCache string

	// Directory where atomfs keeps its working storage
	// e.g. upperdirs and temporary mounts
	ScratchWrites string

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
	storage Storage
	//bootmgr   Bootmgr

	opts     MosOptions
	lockfile *os.File

	Manifest *SysManifest
}

func NewMos(configDir, storeDir string) (*Mos, error) {
	opts := MosOptions{
		StorageType:      AtomfsStorageType,
		ConfigDir:        configDir,
		StorageCache:     storeDir,
		RootDir:          "/",
		LayersReadOnly:   false,
		ManifestReadOnly: false,
		NoHostCerts:      true,
	}

	s, err := NewStorage(opts)
	if err != nil {
		return nil, fmt.Errorf("Error initializing storage")
	}

	mos := &Mos{
		opts:     opts,
		lockfile: nil,
		storage:  s,
	}

	if err := mos.acquireLock(); err != nil {
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
	if opts.RootDir != "/" && !strings.HasPrefix(opts.CaPath, opts.RootDir) {
		opts.CaPath = filepath.Join(opts.RootDir, opts.CaPath)
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
		opts:    opts,
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
	manifest, err := mos.CurrentManifest()
	if err != nil {
		return nil, errors.Wrapf(err, "Failed opening manifest")
	}

	for _, t := range manifest.SysTargets {
		if t.Name == name {
			return t.raw, nil
		}
	}

	return nil, errors.Errorf("Target %s not found", name)
}

// Activate a target (service):
// If it is not yet running then start it.
// If it is already running, but is not at the newest version (i.e. after an
// upgrade), then restart it. (Not fully implemented)
func (mos *Mos) Activate(name string) error {
	t, err := mos.Current(name)
	if err != nil {
		return errors.Wrapf(err, "Failed to get current version of %s", name)
	}

	if t.ServiceType == HostfsService {
		return errors.Errorf("Reboot not yet supported, do it yourself")
	}

	v, err := mos.RunningVersion(t)
	if err != nil {
		return errors.Wrapf(err, "Failed getting running version of %s", name)
	}

	log.Infof("running version is \"%q\" wanted version is %q", v, t.Version)
	if v == t.Version {
		// TODO this is actually broken - what we get is the layer config
		// hash (e.g. ee60af2a371e08a67174387985dd5f270d5596f7265f94f15107889aab454157),
		// but we need the version.  This isn't so bad, we just always restart
		// on activate.

		// latest version already running
		return nil
	}

	if v != "" {
		log.Infof("Stopping target %q", t.ServiceName)
		err = mos.StopTarget(t)
		if err != nil {
			return errors.Wrapf(err, "Failed stopping service %s for update", name)
		}
		log.Infof("Stopped target %q", t.ServiceName)
	}

	err = mos.SetupTargetRuntime(t)
	if err != nil {
		return errors.Wrapf(err, "Error setting up runtime for %s", name)
	}

	if t.ServiceType != ContainerService {
		return nil
	}

	err = mos.startInit(t)
	if err != nil {
		return errors.Wrapf(err, "Error starting %s", name)
	}

	return nil
}

func (mos *Mos) SetupTargetRuntime(t *Target) error {
	log.Debugf("Setting up target %s", t.ServiceName)
	err := mos.storage.SetupTarget(t)
	if err != nil {
		return fmt.Errorf("Failed setting up storage for %s:%s: %w", t.ServiceName, t.Version, err)
	}

	switch t.ServiceType {
	case FsService:
		return mos.startFsOnly(t)
	case HostfsService:
		return nil
	case ContainerService:
		return mos.setupContainerService(t)
	default:
		return fmt.Errorf("Unhandled service type %s", t.ServiceType)
	}
}

func (mos *Mos) GetSystarget(t *Target) (*SysTarget, error) {
	manifest, err := mos.CurrentManifest()
	if err != nil {
		return &SysTarget{}, err
	}
	for _, e := range manifest.SysTargets {
		if e.Name == t.ServiceName {
			return &e, nil
		}
	}

	return &SysTarget{}, fmt.Errorf("No system target found for %s!", t.ServiceName)
}

func (mos *Mos) startFsOnly(t *Target) error {
	src, err := mos.storage.TargetMountdir(t)
	if err != nil {
		return err
	}
	dest := filepath.Join(mos.opts.RootDir, "/mnt/atom", t.ServiceName)
	if err := EnsureDir(dest); err != nil {
		return fmt.Errorf("Unable to create directory %s: %w", dest, err)
	}
	if err = unix.Mount(src, dest, "", unix.MS_BIND, ""); err != nil {
		return err
	}
	return nil
}

func (mos *Mos) setupContainerService(t *Target) error {
	err := mos.writeLxcConfig(t)
	if err != nil {
		return err
	}

	err = mos.writeContainerService(t)
	if err != nil {
		return err
	}

	return nil
}

func (mos *Mos) SetupNetwork(t *Target) ([]string, error) {
	switch t.Network.Type {
	case HostNetwork:
		return []string{"lxc.net.0.type = none"}, nil
	case NoNetwork:
		return []string{"lxc.net.0.type = empty"}, nil
	default:
		return []string{}, fmt.Errorf("Unhandled network type: %s", t.Network.Type)
	}
}

func (mos *Mos) writeLxcConfig(t *Target) error {
	// We are guaranteed to have stopped the container before reaching
	// here
	lxcStateDir := filepath.Join(mos.opts.RootDir, "var/lib/lxc")
	lxcconfigDir := filepath.Join(lxcStateDir, t.ServiceName)
	lxclogDir := filepath.Join(mos.opts.RootDir, "/var/log/lxc")
	if err := os.RemoveAll(lxcconfigDir); err != nil {
		return fmt.Errorf("Failed removing pre-existing container config for %q: %w", t.ServiceName, err)
	}
	if err := EnsureDir(lxcconfigDir); err != nil {
		return fmt.Errorf("Failed creating container config dir: %w", err)
	}
	if err := os.Chmod(lxcStateDir, 0755); err != nil {
		return fmt.Errorf("Failed setting perms on host container configuration directory: %w", err)
	}

	syst, err := mos.GetSystarget(t)
	if err != nil {
		return err
	}

	lxcConf := []string{}

	rfs, err := mos.storage.TargetMountdir(t)
	if err != nil {
		return err
	}

	idmapset, lxcIdrange, err := mos.GetUIDMapStr(t)
	if err != nil {
		return err
	}
	for _, line := range lxcIdrange {
		lxcConf = append(lxcConf, "lxc.idmap = "+line)
	}

	if err := addUidMapping(idmapset); err != nil {
		return err
	}

	if len(syst.OCIConfig.Config.Entrypoint) == 0 || syst.OCIConfig.Config.Entrypoint[0] == "" {
		return fmt.Errorf("No entrypoint defined for %q", t.ServiceName)
	}
	cmd := append(syst.OCIConfig.Config.Entrypoint, syst.OCIConfig.Config.Cmd...)
	for i, c := range cmd {
		if strings.Contains(c, " ") {
			cmd[i] = fmt.Sprintf("%q", c)
		}
	}

	const maxTries int = 10
	count := 0
	canary := filepath.Join(rfs, cmd[0])
	for ; count < maxTries; count++ {
		// make sure squashfuse is ready
		_, err = os.Lstat(canary)
		if err == nil {
			break
		}
		time.Sleep(1 * time.Second)
	}
	if count == maxTries {
		return fmt.Errorf("Timed out waiting for rfs at %q", rfs)
	}
	log.Infof("mountpoint %q is ready after %d seconds", rfs, count)

	if !UidmapIsHost() {
		err = fixupSymlinks(rfs)
		if err != nil {
			return err
		}
	}

	if len(idmapset.Idmap) != 0 {
		err = idmapset.ShiftFile(rfs)
		if err != nil {
			return err
		}
	}
	lxcConf = append(lxcConf, "lxc.rootfs.path = "+rfs)

	netconf, err := mos.SetupNetwork(t)
	if err != nil {
		return err
	}
	lxcConf = append(lxcConf, netconf...)

	lxcConf = append(lxcConf, fmt.Sprintf("lxc.uts.name = %s", t.ServiceName))

	lxcConf = append(lxcConf, fmt.Sprintf("lxc.execute.cmd = %s", strings.Join(cmd, " ")))
	lxcConf = append(lxcConf, "lxc.mount.auto = proc:mixed")
	lxcConf = append(lxcConf, "lxc.log.level = TRACE")
	// XXX TODO the apparmor profile should only be unset if we
	// are running in a confined, nested parent container (for testing).
	lxcConf = append(lxcConf, "lxc.apparmor.profile = unchanged")
	lxcConf = append(lxcConf, fmt.Sprintf("lxc.log.file = %s/%s.log", lxclogDir, t.ServiceName))

	for _, env := range syst.OCIConfig.Config.Env {
		lxcConf = append(lxcConf, fmt.Sprintf("lxc.environment = %s", env))
	}

	// TODO - setup the mounts

	// Write the result
	lxcConfFile := filepath.Join(lxcconfigDir, "config")
	data := []byte(strings.Join(lxcConf, "\n") + "\n")
	err = os.WriteFile(lxcConfFile, data, 0644)
	if err != nil {
		return fmt.Errorf("couldn't write config file %q: %w", lxcConfFile, err)
	}
	err = os.WriteFile("/tmp/lxcconf", data, 0644)
	if err != nil {
		return fmt.Errorf("couldn't write config file %q: %w", "/tmp/lxcconf", err)
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
	log.Infof("RunningVersion: %s has version %q", t.ServiceName, hash)

	return hash, nil
}

func (mos *Mos) StopTarget(t *Target) error {
	unitName := fmt.Sprintf("%s.service", t.ServiceName)
	switch t.ServiceType {
	case ContainerService:
		out, rc := RunCommandWithRc("systemctl", "stop", unitName)
		outs := string(out)
		if rc != 0 && !strings.HasSuffix(outs, "not loaded.\n") {
			return fmt.Errorf("Failed to stop service %s: %s", t.ServiceName, outs)
		}
	case HostfsService:
		return fmt.Errorf("Stopping hostfs is not yet supported.  Please poweroff")
	case FsService:
		mp := filepath.Join(mos.opts.RootDir, "/mnt/atom", t.ServiceName)
		err := unix.Unmount(mp, 0)
		if err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("Unhandled service type: %s", t.ServiceType)
	}

	err := mos.storage.TearDownTarget(t.ServiceName)
	if err != nil {
		return fmt.Errorf("Failed shutting down storage for %s: %w", t.ServiceName, err)
	}

	return nil
}

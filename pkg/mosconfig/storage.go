package mosconfig

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	ispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/apex/log"
	"github.com/opencontainers/umoci"
	"golang.org/x/sys/unix"
	"stackerbuild.io/stacker/pkg/atomfs"
	"stackerbuild.io/stacker/pkg/lib"
	"stackerbuild.io/stacker/pkg/mount"
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

	MountedByHash(target *Target) (string, error)
	TearDownTarget(name string) error
	TargetMountdir(t *Target) (string, error)
	SetupTarget(t *Target) error
	VerifyTarget(t *Target) error

	ImportTarget(srcDir string, target *Target) error
}

func NewStorage(opts MosOptions) (Storage, error) {
	var s Storage
	var e error
	switch opts.StorageType {
	case AtomfsStorageType:
		s, e = NewAtomfsStorage(opts.RootDir, opts.StorageCache, opts.ScratchWrites)
	case PuzzlefsStorageType:
		return nil, fmt.Errorf("Not yet implemented")
	default:
		return nil, fmt.Errorf("Unknown storage type requested")
	}

	return s, e
}

type AtomfsStorage struct {
	RootDir     string
	zotPath     string
	scratchPath string
}

func NewAtomfsStorage(rootDir, zotPath, scratchPath string) (*AtomfsStorage, error) {
	return &AtomfsStorage{
		RootDir:     rootDir,
		zotPath:     zotPath,
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
	if err := EnsureDir(mountpoint); err != nil {
		return func() {}, fmt.Errorf("Failed creating mountpoint %q: %w", mountpoint, err)
	}

	opts := atomfs.MountOCIOpts{
		OCIDir:       filepath.Join(a.zotPath, t.ImagePath),
		MetadataPath: a.metadataPath(),
		Tag:          t.Version,
		Target:       mountpoint,
	}
	if !UidmapIsHost() {
		opts.AllowMissingVerityData = true
	}

	mol, err := atomfs.BuildMoleculeFromOCI(opts)
	if err != nil {
		return func() {}, fmt.Errorf("Failed building atomfs molecule for %#v: %w", opts, err)
	}

	cleanup := func() {
		err := atomfs.Umount(mountpoint)
		if err != nil {
			log.Warnf("unmounting %s failed: %s", mountpoint, err)
		}
	}
	err = mol.Mount(mountpoint)
	if err != nil {
		return cleanup, fmt.Errorf("Failed mounting molecule %#v: %w", mol, err)
	}
	return cleanup, nil
}

func (a *AtomfsStorage) MountWriteable(t *Target, mountpoint string) (func(), error) {
	ropath, err := os.MkdirTemp(a.scratchPath, fmt.Sprintf("%s-scratch-readonly-", t.ServiceName))
	if err != nil {
		return func() {}, fmt.Errorf("Failed creating readonly mountpoint: %w", err)
	}

	roCleanup, err := a.Mount(t, ropath)
	if err != nil {
		os.Remove(ropath)
		return func() {}, fmt.Errorf("Failed creating readonly mount for %#v: %w", t, err)
	}

	workdir, err := os.MkdirTemp(a.scratchPath, fmt.Sprintf("%s-scratch-workdir-", t.ServiceName))
	if err != nil {
		roCleanup()
		os.Remove(ropath)
		return func() {}, fmt.Errorf("Failed creating workdir: %w", err)
	}

	upperdir, err := os.MkdirTemp(a.scratchPath, fmt.Sprintf("%s-scratch-upperdir-", t.ServiceName))
	if err != nil {
		roCleanup()
		os.Remove(ropath)
		os.RemoveAll(workdir)
		return func() {}, fmt.Errorf("Failed creating upperdir: %w", err)
	}

	overlayArgs := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s,userxattr", ropath, upperdir, workdir)
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

func getHashFromOverlay(mountinfo string, mountPoint string) (string, error) {
	mounts, err := mount.ParseMounts(mountinfo)
	if err != nil {
		return "", err
	}

	for _, m := range mounts {
		if m.Target != mountPoint {
			continue
		}

		dirs, err := m.GetOverlayDirs()
		if err != nil {
			return "", fmt.Errorf("Failed getting overlay dirs for mount %+v: %w", m, err)
		}

		// atomix has traditionally used the first layer as the 'hash'
		// field of everything.
		firstDir := dirs[0]
		hash := filepath.Base(firstDir)
		return hash, nil
	}

	return "", nil
}

func (a *AtomfsStorage) MountedByHash(target *Target) (string, error) {
	switch target.ServiceType {
	case "hostfs":
		return getHashFromOverlay("/proc/self/mountinfo", a.RootDir)
	case "fs-only":
		/* see SetupTargetRuntime() */
		return getHashFromOverlay("/proc/self/mountinfo", filepath.Join(a.RootDir, "mnt/atom", target.ServiceName))
	case "container":
		// container services are lxc containers, which may or may not
		// have their rootfs visible in this mount namespace. let's
		// look at the specific mountinfo for the container just to be
		// sure.
		out, rc := RunCommandWithRc("lxc-info", "-H", "-n", target.ServiceName, "-s")
		if rc != 0 {
			/* if the service didn't previously exist, it's ok for lxc-ls to fail */
			return "", nil
		}
		if strings.TrimSpace(string(out)) != "RUNNING" {
			return "", nil
		}
		out, rc = RunCommandWithRc("lxc-info", "-H", "-n", target.ServiceName, "-p")
		if rc != 0 {
			/* if the service didn't previously exist, it's ok for lxc-ls to fail */
			return "", nil
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(out)))
		if err != nil {
			return "", fmt.Errorf("couldn't get pid from %s: %w", strings.TrimSpace(string(out)), err)
		}

		return getHashFromOverlay(fmt.Sprintf("/proc/%d/mountinfo", pid), "/")
	default:
		return "", fmt.Errorf("couldn't determine mountpoint for %s (%s)", target.ServiceName, target.ServiceType)
	}
}

func (a *AtomfsStorage) SetupTarget(t *Target) error {
	mp := filepath.Join(a.scratchPath, "roots", t.ServiceName)
	mounted, err := IsMountpoint(mp)
	if err != nil {
		return fmt.Errorf("Failed checking whether %q is mounted: %w", mp, err)
	}
	if mounted {
		err := atomfs.Umount(mp)
		if err != nil {
			return err
		}
	}

	err = EnsureDir(mp)
	if err != nil {
		return fmt.Errorf("Failed creating mountpoint %q: %w", mp, err)
	}

	if t.ServiceType == "container" {
		// For containers, we have to make this writeable to support
		// uid shifting.  We can un-do this if/when we can use id mapped
		// mounts.
		// XXX TODO we should probably, therefore, umount this after
		// every service stop.
		_, err = a.MountWriteable(t, mp)
	} else {
		_, err = a.Mount(t, mp)
	}
	if err != nil {
		return fmt.Errorf("Failed mounting %s:%s to %q: %w", t.ServiceName, t.Version, mp, err)
	}

	return nil
}

// We mount a readonly copy of the fs under $scratch-writes/roots/$target.
// A container service will want to set lxc.rootfs.path = that, while an
// fs-only service will simply want to do an overlay rw mount onto
// /mnt/atom/$target
func (a *AtomfsStorage) TargetMountdir(t *Target) (string, error) {
	return filepath.Join(a.scratchPath, "roots", t.ServiceName), nil
}

func (a *AtomfsStorage) TearDownTarget(name string) error {
	log.Warnf("tearing down %q", name)
	mp := filepath.Join(a.scratchPath, "roots", name)
	mounted, err := IsMountpoint(mp)
	if err != nil {
		return fmt.Errorf("Failed checking whether %q is mounted: %w", mp, err)
	}
	if !mounted {
		return nil
	}

	err = atomfs.Umount(mp)
	if err != nil {
		return fmt.Errorf("atomfs umount of %q failed: %w", mp, err)
	}
	return err
}

func pickOciOrZot(inDir, inName, inVersion string) (ocidir, name string, err error) {
	err = nil
	if PathExists(filepath.Join(inDir, "index.json")) {
		// simple oci layout
		ocidir = inDir
		name = inName
		if inVersion != "" {
			name = name + ":" + inVersion
		}
		return
	}
	// local zot layout
	ocidir = filepath.Join(inDir, inName)
	name = inVersion
	if !PathExists(filepath.Join(ocidir, "index.json")) {
		err = fmt.Errorf("No image %q:%q under %q", inName, inVersion, inDir)
	}
	return
}

func (a *AtomfsStorage) VerifyTarget(t *Target) error {
	ocidir, name, err := pickOciOrZot(a.zotPath, t.ImagePath, t.Version)
	if err != nil {
		return err
	}

	oci, err := umoci.OpenLayout(ocidir)
	if err != nil {
		return fmt.Errorf("Failed reading OCI manifest for %s: %w", t.ImagePath, err)
	}
	defer oci.Close()

	descriptorPaths, err := oci.ResolveReference(context.Background(), name)
	if err != nil {
		return err
	}

	if len(descriptorPaths) != 1 {
		return fmt.Errorf("bad descriptor %s for %#v in %q", t.Version, t, ocidir)
	}

	blob, err := oci.FromDescriptor(context.Background(), descriptorPaths[0].Descriptor())
	if err != nil {
		return err
	}
	defer blob.Close()

	if blob.Descriptor.MediaType != ispec.MediaTypeImageManifest {
		return fmt.Errorf("descriptor does not point to a manifest: %s", blob.Descriptor.MediaType)
	}

	realsum := blob.Descriptor.Digest.Encoded()
	if realsum != t.ManifestHash {
		return fmt.Errorf("Hash is %q, should be %q", realsum, t.ManifestHash)
	}

	return nil
}

// Import a target's storage.  src is the install media base
// directory, under which we expect either oci or zot.
// src could also be a remote zot server, but that's not yet
// implemented.
func (a *AtomfsStorage) ImportTarget(src string, target *Target) error {
	if src == "" {
		return fmt.Errorf("remote image copy not yet implemented")
	}
	zotDir := filepath.Join(src, "zot")
	ociDir := filepath.Join(src, "oci")
	var err error
	switch {
	case PathExists(ociDir):
		err = a.copyLocalOci(ociDir, target)
	case PathExists(zotDir):
		err = a.copyLocalZot(zotDir, target)
	default:
		err = fmt.Errorf("no oci or zot storage found under %s", src)
	}

	if err != nil {
		return fmt.Errorf("Error extracting target %#v: %w", target, err)
	}

	return nil
}

func (a *AtomfsStorage) copyLocalZot(zotSourceDir string, target *Target) error {
	layerDir := filepath.Join(zotSourceDir, target.ImagePath)
	src := fmt.Sprintf("oci:%s:%s", layerDir, target.Version)
	tpath := filepath.Join(a.zotPath, target.ImagePath)
	if err := EnsureDir(tpath); err != nil {
		return fmt.Errorf("Failed creating local zot directory %q: %w", tpath, err)
	}
	dest := fmt.Sprintf("oci:%s:%s", tpath, target.Version)

	log.Infof("copying %q:%s from local zot ('%s') into zot as '%s'", target.ImagePath, target.Version, src, dest)

	copyOpts := lib.ImageCopyOpts{Src: src, Dest: dest, Progress: os.Stdout}
	if err := lib.ImageCopy(copyOpts); err != nil {
		return fmt.Errorf("failed copying layer %v: %w", target, err)
	}

	return nil
}

func (a *AtomfsStorage) copyLocalOci(ociDir string, target *Target) error {
	src := fmt.Sprintf("oci:%s:%s", ociDir, target.ServiceName)
	tpath := filepath.Join(a.zotPath, target.ImagePath)
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

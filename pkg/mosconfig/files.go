package mosconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/apex/log"
	ispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"github.com/project-machine/mos/pkg/trust"
	"github.com/project-machine/mos/pkg/utils"

	"machinerun.io/disko"
	"machinerun.io/disko/partid"
)

// Update can be full, meaning all existing Targets are replaced, or
// partial, meaning those in the install manifest are installed or
// replaced, but any other Targets on the system remain.

// An install manifest is a shipped, signed manifest of Targets.
// A system manifest is an intermediary list of targets actually
// installed on the system.  In a full install, the system manifest
// will contain the full set of targets in the install manifest.  On
// a partial install, the system manifest contains the new targets as
// well as any pre-existing targets which the new install manifest
// did not replace.

type UpdateType string

const (
	PartialUpdate UpdateType = "partial"
	FullUpdate    UpdateType = "complete"
)

func ParseUpdateType(t string) (UpdateType, error) {
	switch t {
	case "partial":
		return PartialUpdate, nil
	case "complete":
		return FullUpdate, nil
	default:
		return "", fmt.Errorf("Unknown update type")
	}
}

const CurrentInstallFileVersion = 1

// Service networking.
// Only host network supported right now.
// To do: simple/nat, CNI
type TargetNetworkType string

const (
	HostNetwork   TargetNetworkType = "host"
	NoNetwork     TargetNetworkType = "none"
	SimpleNetwork TargetNetworkType = "simple"
)

// Target Network configuration
// Type dictates which type
// Address is an ipv4 or ipv6 address.
// Ports are ipfilter rules to allow inbound masq.
//
// We will likely want to change this to an array of
// nics, like lxc.net.[i].*.  But for now let's just
// support one
type TargetNetwork struct {
	Type     TargetNetworkType `json:"type" yaml:"type"`
	Address  string            `json:"ipv4" yaml:"ipv4"`
	Address6 string            `json:"ipv6" yaml:"ipv6"`
	Ports    []SimplePort      `json:"ports" yaml:"ports"`
}

// Service type defines how a service is run.
// Hostfs is the "root filesystem".
// Container services run in lxc containers.
// FsService (fs-only) only offers a filesystem that can
// be mounted for user by others.
type ServiceType string

const (
	HostfsService    ServiceType = "hostfs"
	ContainerService ServiceType = "container"
	FsService        ServiceType = "fs-only"
)

type TargetStorage struct {
	Dest  string `json:"dest" yaml:"dest"`
	Label string `json:"label" yaml:"label"`
}
type TargetStorageList []TargetStorage

// Target defines a single service.  This includes the rootfs
// and every container and fs-only service.
// NSGroup is a user namespace group.  Two services both in
// NSGroup 'ran' will have the same uid mapping.  A service
// in NSGroup "none" (or "") runs in the host uid network.
type Target struct {
	ServiceName string            `json:"service_name"` // name of target
	Version     string            `json:"version"`      // docker or oci version tag
	ServiceType ServiceType       `json:"service_type"`
	Network     TargetNetwork     `json:"network"`
	NSGroup     string            `json:"nsgroup"`
	Digest      string            `json:"digest"`
	Storage     TargetStorageList `json:"storage"`
	Size        int64             `json:"size"`
}
type InstallTargets []Target

func (t *Target) NeedsIdmap() bool {
	return needsIdmap(t.NSGroup)
}
func needsIdmap(nsgroup string) bool {
	return nsgroup != "" && nsgroup != "none"
}

// Note - Storage is an interface, an implementation detail
// to abstract atomfs vs puzzlefs etc.  So for the 'storage'
// information in manifest.yaml, we use StorageItem and
// StorageList.

// StorageItem is a request for a volume to be mounted into
// a Target.
type StorageItem struct {
	Label      string `json:"label" yaml:"label"`
	Persistent bool   `json:"persistent" yaml:"persistent"`
	NSGroup    string `json:"nsgroup" yaml:"nsgroup"`
	Size       uint64 `json:"size" yaml:"size"` // size in Mib
}

func (i *StorageItem) Delete(allDisks disko.DiskSet, mysys disko.System) error {
	for _, d := range allDisks {
		for _, p := range d.Partitions {
			if p.Name == i.Label {
				err := mysys.DeletePartition(d, p.Number)
				if err != nil {
					return err
				}
				return nil
			}
		}
	}
	return nil
}

func (i *StorageItem) IsReserved() bool {
	for _, n := range []string{"esp", "machine-config", "machine-store", "machine-scratch"} {
		if i.Label == n {
			return true
		}
	}
	return false
}

const mib, gib = disko.Mebibyte, disko.Mebibyte * 1024

// Create a user-requested storage item if it does not yet exist.
// We accept the mos struct so we can find its nsgroup (uid mapping)
func (i *StorageItem) Create(mos *Mos, allDisks disko.DiskSet, mysys disko.System) error {
	size := i.Size * disko.Mebibyte
	for _, d := range allDisks {
		fslist := d.FreeSpacesWithMin(size)
		if len(fslist) == 0 {
			continue
		}
		num := uint(0)
		for i := uint(1); i <= 128; i++ {
			if _, ok := d.Partitions[i]; !ok {
				num = i
				break
			}
		}
		if num == 0 {
			return errors.Errorf("No free partition numbers")
		}
		freespace := fslist[0]
		start := freespace.Start
		p := disko.Partition{
			Start:  start,
			Last:   start + size - 1,
			Number: num,
			ID:     disko.GenGUID(),
			Type:   partid.LinuxFS,
			Name:   i.Label,
		}
		if err := mysys.CreatePartition(d, p); err != nil {
			return errors.Wrapf(err, "Failed creating storage %#v", i)
		}
		dev := filepath.Join("/dev", pathForPartition(d.Name, p.Number))
		cmd := []string{"mkfs.ext4", "-F", dev}
		if err := utils.RunCommand(cmd...); err != nil {
			return errors.Wrapf(err, "Failed creating fs on %#v", i)
		}

		// mount
		dest := filepath.Join("/storage", i.Label)
		if err := utils.EnsureDir(dest); err != nil {
			return errors.Wrapf(err, "Failed creating mount dir %q", dest)
		}
		if err := syscall.Mount(dev, dest, "ext4", 0, ""); err != nil {
			return errors.Wrapf(err, "Failed mounting %#v", i)
		}

		idmapset, _, err := mos.GetUIDMapStr(i.NSGroup)
		if err != nil {
			return err
		}
		if len(idmapset.Idmap) != 0 {
			if err := idmapset.ShiftFile(dest); err != nil {
				return errors.Wrapf(err, "Failed shifting %q to %#v", dest, idmapset.Idmap)
			}
		}
		log.Infof("Created and mounted %#v onto %q", i, dest)
		return nil
	}
	return errors.Errorf("Failed to find free space for %#v", i)
}

type StorageList []StorageItem

func (s StorageList) Contains(n StorageItem) bool {
	for _, i := range s {
		if i.Label == n.Label {
			return true
		}
	}
	return false
}

// This describes an install manifest
type InstallFile struct {
	Version    int            `json:"version"`
	Product    string         `json:"product"`
	Storage    StorageList    `json:"storage"`
	Targets    InstallTargets `json:"targets"`
	UpdateType UpdateType     `json:"update_type"`
}

// Note we only do combined uid+gid ranges, range 65536, and only starting at
// container id 0.
type IdmapSet struct {
	Name   string `json:"idmap-name"` // This is the NSGroup specified in target
	Hostid int64  `json:"hostid"`
}

// SysTarget exists as an intermediary between a 'system manifest'
// and an 'install manifest'
type SysTarget struct {
	Name   string `json:"name"`   // the name of the target
	Source string `json:"source"` // the content address manifest file defining it

	raw         *Target
	OCIManifest ispec.Manifest
	OCIConfig   ispec.Image
}
type SysTargets []SysTarget

func (s *SysTargets) Contains(needle SysTarget) bool {
	for _, t := range *s {
		if t.Name == needle.Name {
			return true
		}
	}
	return false
}

type SysManifest struct {
	// Persistent stored information
	UidMaps    []IdmapSet  `json:"uidmaps"`
	SysTargets []SysTarget `json:"targets"`
	Storage    StorageList `json:"storage"`

	// Runtime information
	DefaultNic string
	UsedPorts  map[uint]string   // map of hostport -> running target name
	IpAddrs    map[string]string // map of ip4 ip6 addr -> running target name
}

func (sm *SysManifest) GetTarget(target string) (*SysTarget, error) {
	for _, t := range sm.SysTargets {
		if t.Name == target {
			return &t, nil
		}
	}
	return nil, errors.Errorf("No such target: %q", target)
}

func (af *InstallFile) Validate() error {
	if af.Product == "" {
		return fmt.Errorf("Must specify a product")
	}

	if af.Version > CurrentInstallFileVersion || af.Version < 1 {
		return fmt.Errorf("unsupported atomix file version: %d", af.Version)
	}

	if err := af.Targets.Validate(); err != nil {
		return err
	}

	if af.UpdateType == "" {
		af.UpdateType = PartialUpdate
	}

	return nil
}

func (af *InstallFile) GetTarget(target string) (*Target, error) {
	for _, t := range af.Targets {
		if t.ServiceName == target {
			return &t, nil
		}
	}
	return nil, errors.Errorf("No such target: %q", target)
}

func simpleParseInstall(manifestPath string) (InstallFile, error) {
	bytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return InstallFile{}, errors.Wrapf(err, "Failed reading manifest")
	}
	var manifest InstallFile
	err = json.Unmarshal(bytes, &manifest)
	if err != nil {
		return InstallFile{}, errors.Wrapf(err, "Failed parsing manifest")
	}

	return manifest, nil
}

// Verify an install.json manifest.  Return the parsed manifest.
// @is is the InstallSource of the install.json.
// @s is the storage driver, currently always an atomfs.
func ReadVerifyInstallManifest(is InstallSource, capath string, s Storage) (InstallFile, error) {
	bytes, err := os.ReadFile(is.FilePath)
	if err != nil {
		return InstallFile{}, fmt.Errorf("Failed reading manifest: %w", err)
	}

	if err := trust.VerifyManifest(bytes, is.SignPath, is.CertPath, capath); err != nil {
		return InstallFile{}, err
	}

	var manifest InstallFile
	err = json.Unmarshal(bytes, &manifest)
	if err != nil {
		return InstallFile{}, fmt.Errorf("Failed parsing manifest: %w", err)
	}

	// We've verified the install.json contents.  Now verify that the container
	// image manifest files pointed to have not been altered.
	for _, t := range manifest.Targets {
		if is.ocirepo != nil {
			// Import the layer into our zot store.
			// We could consider deleting the layer if VerifyTarget fails below.
			// This is not terribly important as nothing will use it,
			// unless there's a manifest which is properly signed which refers
			// to it, in which case we'll regret having deleted it...
			src := fmt.Sprintf("docker://%s/mos:%s", is.ocirepo.addr, dropHashAlg(t.Digest))
			if err := s.ImportTarget(src, &t); err != nil {
				return InstallFile{}, err
			}
		}

		if err := s.VerifyTarget(&t); err != nil {
			return InstallFile{}, fmt.Errorf("Bad manifest hash for %q: %w", t.ServiceName, err)
		}
	}

	if err := manifest.Validate(); err != nil {
		return InstallFile{}, err
	}

	return manifest, nil
}

func (ts InstallTargets) Validate() error {
	for _, t := range ts {
		if t.ServiceName == "" {
			return fmt.Errorf("Target field 'name' cannot be empty: %#v", t)
		}

		if t.Version == "" {
			return fmt.Errorf("Target %s cannot have empty version", t.ServiceName)
		}

		if !t.ValidateNetwork() {
			return fmt.Errorf("Target %s has bad network: %#v", t.ServiceName, t.Network)
		}
	}

	return nil
}

// The import manifest (manifest.yaml) which the user writes,
// and which mosb converts into an install.json.

type ImportFile struct {
	Version    int         `yaml:"version"`
	Product    string      `yaml:"product"`
	Storage    StorageList `yaml:"storage"`
	Targets    UserTargets `yaml:"targets"`
	UpdateType UpdateType  `yaml:"update_type"`
}

func (i *ImportFile) HasTarget(name string) bool {
	for _, t := range i.Targets {
		if t.ServiceName == name {
			return true
		}
	}
	return false
}

func (i *ImportFile) CompleteTargets(keyProject string) (UserTargets, error) {
	if !i.HasTarget("hostfs") {
		s := fmt.Sprintf("docker://zothub.io/machine/bootkit/demo-target-rootfs:%s-squashfs", trust.RelVersion)
		newT := UserTarget{
			ServiceName: "hostfs",
			ServiceType: "hostfs",
			Source:      s,
			Version:     trust.BootkitVersion,
			Network:     TargetNetwork{Type: HostNetwork},
		}
		i.Targets = append(i.Targets, newT)
	}
	if !i.HasTarget("bootkit") {
		bootkitDir, err := bootkitDir(keyProject)
		if err != nil {
			return UserTargets{}, err
		}
		newT := UserTarget{
			ServiceName: "bootkit",
			Source:      fmt.Sprintf("oci:%s/oci:bootkit-squashfs", bootkitDir),
			Version:     "1.0.0",
			ServiceType: "fs-only",
			Network:     TargetNetwork{Type: HostNetwork},
		}
		i.Targets = append(i.Targets, newT)
	}
	return i.Targets, nil
}

type UserTarget struct {
	ServiceName string            `yaml:"service_name"` // name of target
	Source      string            `yaml:"source"`       // docker url from which to fetch
	Version     string            `yaml:"version"`      // A version for internal use.
	Storage     TargetStorageList `yaml:"storage"`
	ServiceType ServiceType       `yaml:"service_type"`
	Network     TargetNetwork     `yaml:"network"`
	NSGroup     string            `yaml:"nsgroup"`
	Digest      string            `yaml:"digest"`
	Size        int64             `yaml:"size"`
}
type UserTargets []UserTarget

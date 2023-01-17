package mosconfig

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	ispec "github.com/opencontainers/image-spec/specs-go/v1"
	"gopkg.in/yaml.v2"
)

// An ImageType can be either an ISO or a Zap layer.
type ImageType string

const (
	ISO ImageType = "iso"
	ZAP ImageType = "zap"
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

const CurrentInstallFileVersion = 1

type MountSpec struct {
	Source  string `yaml:"source"`
	Dest    string `yaml:"dest"`
	Options string `yaml:"options"`
}

// Only host network supported right now.
// To do: simple/nat, CNI
type TargetNetworkType string

const (
	HostNetwork TargetNetworkType = "host"
	NoNetwork   TargetNetworkType = "none"
)

type TargetNetwork struct {
	Type TargetNetworkType `yaml:"type"`
}

type ServiceType string

const (
	HostfsService    ServiceType = "hostfs"
	ContainerService ServiceType = "container"
	FsService        ServiceType = "fs-only"
)

type Target struct {
	SourceLayer  string        `yaml:"layer"`
	Name         string        `yaml:"name"`     // name of target
	Fullname     string        `yaml:"fullname"` // full zot path
	Version      string        `yaml:"version"`  // docker or oci version tag
	ServiceType  ServiceType   `yaml:"service_type"`
	Network      TargetNetwork `yaml:"network"`
	NSGroup      string        `yaml:"nsgroup"`
	Mounts       []*MountSpec  `yaml:"mounts"`
	ManifestHash string        `yaml:"manifest_hash"`
}
type InstallTargets []Target

func (t *Target) NeedsIdmap() bool {
	return t.NSGroup != "" && t.NSGroup != "none"
}

// This describes an install manifest
type InstallFile struct {
	Version     int            `yaml:"version"`
	ImageType   ImageType      `yaml:"image_type"`
	Product     string         `yaml:"product"`
	Hooks       string         `yaml:"hooks"`
	Targets     InstallTargets `yaml:"targets"`
	UpdateType  UpdateType     `yaml:"update_type"`
	StorageType StorageType    `yaml:"storage_type"`
	// The original file contents, exactly what was signed
	original string
}

// Note we only do combined uid+gid ranges, range 65536, and only starting at
// container id 0.
type IdmapSet struct {
	Name   string `yaml:"idmap-name"` // This is the NSGroup specified in target
	Hostid int64  `yaml:"hostid"`
}

// SysTarget exists as an intermediary between a 'system manifest'
// and an 'install manifest'
type SysTarget struct {
	Name   string `yaml:"name"`   // the name of the target
	Source string `yaml:"source"` // the content address manifest file defining it

	raw         *Target
	OCIManifest ispec.Manifest
	OCIConfig   ispec.Image
}
type SysTargets []SysTarget

func (s *SysTargets) Contains (needle SysTarget) (SysTarget, bool) {
	for _, t := range *s {
		if t.Name == needle.Name {
			return t, true
		}
	}
	return SysTarget{}, false
}

type SysManifest struct {
	UidMaps    []IdmapSet  `yaml:"uidmaps"`
	SysTargets []SysTarget `yaml:"targets"`
}

func NewInstallFile(p string) (*InstallFile, error) {
	content, err := ioutil.ReadFile(p)
	if err != nil {
		return nil, err
	}

	af := &InstallFile{original: string(content)}
	if err := yaml.Unmarshal(content, af); err != nil {
		return nil, err
	}

	if af.Product == "" {
		return nil, fmt.Errorf("Must specify a product")
	}

	if af.Version > CurrentInstallFileVersion || af.Version < 1 {
		return nil, fmt.Errorf("unsupported atomix file version: %d", af.Version)
	}

	err = af.Targets.Validate()
	if err != nil {
		return nil, err
	}

	// Make all the paths relative to the location of atomix.yaml if
	// they're relative.
	if af.Hooks != "" && !filepath.IsAbs(af.Hooks) {
		af.Hooks = filepath.Join(filepath.Dir(p), af.Hooks)
	}

	if af.UpdateType == "" {
		af.UpdateType = PartialUpdate
	}

	return af, nil
}

func (ts InstallTargets) Validate() error {
	for _, t := range ts {
		if len(strings.Split(t.SourceLayer, ":")) < 2 {
			return fmt.Errorf("invalid source format: %s", t.SourceLayer)
		}

		if t.Name == "" {
			return fmt.Errorf("Target field 'name' cannot be empty: %#v", t)
		}

		if t.Version == "" {
			return fmt.Errorf("Target %s cannot have empty version", t.Name)
		}

		if !t.ValidateNetwork() {
			return fmt.Errorf("Target %s has bad network: %#v", t.Name, t.Network)
		}
	}

	return nil
}

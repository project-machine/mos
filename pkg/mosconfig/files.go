package mosconfig

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"
)

type ImageType string

const (
	ISO ImageType = "iso"
	ZAP ImageType = "zap"
)

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

type Target struct {
	SourceLayer    string       `yaml:"layer"`
	Name           string       `yaml:"name"`      // name of target
	Fullname       string       `yaml:"fullname"`  // full zot path
	Version        string       `yaml:"version"`   // docker or oci version tag
	ServiceType    string       `yaml:"service_type"`
	Network        string       `yaml:"network"`
	NSGroup        string       `yaml:"nsgroup"`
	Mounts         []*MountSpec `yaml:"mounts"`
	ManifestHash   string       `yaml:"manifest_hash"`
}
type InstallTargets []Target

type StorageType string

// Only atomfs is supported right now
const (
	AtomfsStorageType StorageType = "atomfs"
)

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
	}

	return nil
}

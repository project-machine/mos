package mosconfig

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	ispec "github.com/opencontainers/image-spec/specs-go/v1"
	stackeroci "stackerbuild.io/stacker/pkg/oci"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/opencontainers/umoci"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

// Check that the product cert was signed by the global puzzleos cert
// This version can be used by outside callers, like atomix extract-soci
// Note that this version does not verify product pid.
func VerifyCert(cert []byte, caPath string) error {
	paths := []string{
		"/factory/secure/manifestCA.pem",
		"/factory/secure/layerCA.pem",
		"/manifestCA.pem",
		"/layerCA.pem",
	}
	if caPath != "" {
		paths = append(paths, caPath)
	}

	var rootBytes []byte
	var err error
	for _, p := range paths {
		rootBytes, err = os.ReadFile(p)
		if err == nil || !os.IsNotExist(err) {
			break
		}

	}
	if err != nil {
		return fmt.Errorf("Failed reading OCI signing CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(rootBytes) {
		return fmt.Errorf("Failed adding cert from OCI signing CA")
	}

	block, _ := pem.Decode(cert)
	if block == nil {
		return fmt.Errorf("Failed to parse manifest-signing certificate PEM: %w", err)
	}
	parsedCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("Failed reading certificate from manifest: %w", err)
	}

	opts := x509.VerifyOptions{
		Roots:     pool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}
	_, err = parsedCert.Verify(opts)
	if err != nil {
		return fmt.Errorf("OCI signing certificate verification failed: %w", err)
	}
	return nil
}

// Only used during first install.  Create a new $config/manifest.git/
func (mos *Mos) initManifest(manifestPath, manifestCert, manifestCA, configPath string) error {
	shaSum, err := ShaSum(manifestPath)
	if err != nil {
		return fmt.Errorf("Failed calculating shasum: %w", err)
	}

	dir := filepath.Join(configPath, "manifest.git")
	if PathExists(dir) {
		return fmt.Errorf("manifest already exists, chickening out!")
	}

	r, err := git.PlainInit(dir, false)
	if err != nil {
		return fmt.Errorf("Failed initializing git: %w", err)
	}

	w, err := r.Worktree()
	if err != nil {
		return fmt.Errorf("Failed opening git worktree: %w", err)
	}

	cf, err := ReadVerifyManifest(manifestPath, manifestCert, manifestCA, "", mos.storage)
	if err != nil {
		return fmt.Errorf("Failed verifying signature on %s: %w", manifestPath, err)
	}

	sFile := fmt.Sprintf("%s.yaml.signed", shaSum)
	dest := filepath.Join(dir, sFile)
	err = CopyFileBits(manifestPath+".signed", dest)
	if err != nil {
		return fmt.Errorf("Failed copying install manifest: %w", err)
	}
	_, err = w.Add(sFile)
	if err != nil {
		return fmt.Errorf("Git file add for manifest signature (%q) failed: %w", sFile, err)
	}

	mFile := fmt.Sprintf("%s.yaml", shaSum)
	dest = filepath.Join(dir, mFile)
	err = CopyFileBits(manifestPath, dest)
	if err != nil {
		return fmt.Errorf("Failed copying install manifest: %w", err)
	}
	_, err = w.Add(mFile)
	if err != nil {
		return fmt.Errorf("Git file add for manifest failed: %w", err)
	}

	pFile := fmt.Sprintf("%s.pem", shaSum)
	dest = filepath.Join(dir, pFile)
	err = CopyFileBits(manifestCert, dest)
	if err != nil {
		return fmt.Errorf("Failed copying manifest Cert: %w", err)
	}
	_, err = w.Add(pFile)
	if err != nil {
		return fmt.Errorf("Git file add for cert failed: %w", err)
	}

	dest = filepath.Join(dir, "manifest.yaml")
	targets := SysTargets{}
	uidmaps := []IdmapSet{}

	for _, t := range cf.Targets {
		newT := SysTarget{
			Name:   t.ServiceName,
			Source: mFile,
		}
		targets = append(targets, newT)

		uidmaps = addUIDMap([]IdmapSet{}, uidmaps, t)
	}

	sysmanifest := SysManifest{
		UidMaps:    uidmaps,
		SysTargets: targets,
	}

	bytes, err := yaml.Marshal(&sysmanifest)
	if err != nil {
		return fmt.Errorf("Failed marshalling the system manifest")
	}

	err = os.WriteFile(dest, bytes, 0640)
	if err != nil {
		return fmt.Errorf("Failed writing system manifest: %w", err)
	}
	_, err = w.Add("manifest.yaml")
	if err != nil {
		return fmt.Errorf("Git file add for system manifest failed: %w", err)
	}
	commitOpts := &git.CommitOptions{
		Author:    defaultSignature(),
		Committer: defaultSignature(),
	}
	_, err = w.Commit("Initial commit", commitOpts)
	if err != nil {
		return fmt.Errorf("Failed committing to git")
	}
	return nil
}

func defaultSignature() *object.Signature {
	when := time.Now()
	return &object.Signature{
		Name:  "machine",
		Email: "root@machine.local",
		When:  when,
	}
}

func (mos *Mos) ReadTargetManifest(t *Target) (ispec.Manifest, ispec.Image, error) {
	emptyM := ispec.Manifest{}
	emptyC := ispec.Image{}
	ociDir := filepath.Join(mos.opts.StorageCache, t.ZotPath)
	oci, err := umoci.OpenLayout(ociDir)
	if err != nil {
		return emptyM, emptyC, fmt.Errorf("Failed reading OCI manifest for %s: %w", t.ZotPath, err)
	}
	defer oci.Close()

	ociManifest, err := stackeroci.LookupManifest(oci, t.Version)
	if err != nil {
		return emptyM, emptyC, err
	}

	ociConfig, err := stackeroci.LookupConfig(oci, ociManifest.Config)
	if err != nil {
		return emptyM, emptyC, err
	}

	return ociManifest, ociConfig, nil
}

func (mos *Mos) CurrentManifest() (*SysManifest, error) {
	if mos.Manifest != nil {
		return mos.Manifest, nil
	}

	dir := filepath.Join(mos.opts.ConfigDir, "manifest.git")
	clonedir, err := os.MkdirTemp("", "verify")
	if err != nil {
		return nil, errors.Wrapf(err, "Error making tempdir")
	}
	defer os.RemoveAll(clonedir)

	r, err := git.PlainClone(clonedir, false,
		&git.CloneOptions{
			URL:           dir,
			ReferenceName: plumbing.Master,
			SingleBranch:  true,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("Error opening the manifest git tree at %q: %w", dir, err)
	}

	w, err := r.Worktree()
	if err != nil {
		return nil, fmt.Errorf("Error getting worktree")
	}

	cOpts := git.CheckoutOptions{
		Branch: plumbing.Master,
		Force:  true,
		Create: false,
	}
	err = w.Checkout(&cOpts)
	if err != nil {
		return nil, fmt.Errorf("Git checkout failed: %w", err)
	}

	f, err := os.Open(filepath.Join(clonedir, "manifest.yaml"))
	if err != nil {
		return nil, fmt.Errorf("Error opening manifest: %w", err)
	}
	defer f.Close()
	contents, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("Failed reading manifest: %w", err)
	}
	var sysmanifest SysManifest
	err = yaml.Unmarshal(contents, &sysmanifest)
	if err != nil {
		return nil, fmt.Errorf("Failed parsing manifest: %w", err)
	}

	manifests := make(map[string]InstallFile)
	ret := SysTargets{}
	for _, t := range sysmanifest.SysTargets {
		h := t.Source
		s, err := mos.readInstallManifest(clonedir, manifests, h)
		if err != nil {
			return nil, errors.Wrapf(err, "Error reading install manifest for %#v", t)
		}
		raw, ok := findTarget(s, t.Name)
		if !ok {
			return nil, fmt.Errorf("target %s not found in %s", t.Name, h)
		}
		t.raw = raw

		t.OCIManifest, t.OCIConfig, err = mos.ReadTargetManifest(t.raw)
		if err != nil {
			return nil, fmt.Errorf("Target manifest not found for %#v: %w", t, err)
		}

		ret = append(ret, t)
	}
	sysmanifest.SysTargets = ret

	mos.Manifest = &sysmanifest

	return &sysmanifest, nil
}

func findTarget(cf InstallFile, name string) (*Target, bool) {
	for _, t := range cf.Targets {
		if t.ServiceName == name {
			return &t, true
		}
	}
	return &Target{}, false
}

// readInstallManifest is used while loading the current, active
// install manifest.
func (mos *Mos) readInstallManifest(gitdir string, l map[string]InstallFile, yName string) (InstallFile, error) {
	r, ok := l[yName]
	if ok {
		return r, nil
	}

	pemName := strings.TrimSuffix(yName, ".yaml") + ".pem"

	tmpd, err := os.MkdirTemp("", "verify")
	if err != nil {
		return InstallFile{}, fmt.Errorf("Failed creating a tempdir: %w", err)
	}
	defer os.RemoveAll(tmpd)

	manifest, err := ReadVerifyManifest (
		filepath.Join(gitdir, yName),
		filepath.Join(gitdir, pemName),
		mos.opts.CaPath,
		"",
		mos.storage)
	if err != nil {
		return InstallFile{}, errors.Wrapf(err, "Failed verifying signature for target %q", yName)
	}

	l[yName] = manifest
	return manifest, nil
}

func (mos *Mos) UpdateManifest(manifest *SysManifest, newmanifest *SysManifest, newdir string) error {
	// Check out a new branch, copy over each required install.yaml
	// from the old manifest, and the files from the new install.

	// TODO - upon failure we should restore the old git branch

	mPath := filepath.Join(mos.opts.ConfigDir, "manifest.git")
	repo, err := git.PlainOpen(mPath)
	if err != nil {
		return fmt.Errorf("Failed opening manifest git repo at %q: %w", mPath, err)
	}

	files, err := os.ReadDir(mPath)
	if err != nil {
		return fmt.Errorf("Failed reading manifest directory: %w", err)
	}

	w, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("Failed opening manifest repo: %w", err)
	}

	// Copy any needed source yamls into our tempdir
	for _, t := range manifest.SysTargets {
		f := t.Source
		if PathExists(filepath.Join(newdir, f)) {
			continue
		}
		base := strings.TrimSuffix(f, ".yaml")
		for _, ext := range []string{".yaml", ".yaml.signed", ".pem"} {
			fName := base + ext
			src := filepath.Join(mPath, fName)
			dest := filepath.Join(newdir, fName)
			if err := CopyFileBits(src, dest); err != nil {
				return fmt.Errorf("Failed copying %q out of system manifest repo: %w", src, err)
			}
		}
	}

	// Remove all files from git index
	for _, f := range files {
		if _, err := w.Remove(f.Name()); err != nil {
			return fmt.Errorf("Failed removing %q from previous manifest: %w", f.Name(), err)
		}
	}

	// And copy back the files we need
	for _, t := range newmanifest.SysTargets {
		f := t.Source
		if PathExists(filepath.Join(mPath, f)) {
			continue
		}
		base := strings.TrimSuffix(f, ".yaml")
		for _, ext := range []string{".yaml", ".yaml.signed", ".pem"} {
			fName := base + ext
			src := filepath.Join(newdir, fName)
			dest := filepath.Join(mPath, fName)
			if err := CopyFileBits(src, dest); err != nil {
				return fmt.Errorf("Failed copying %q to system manifest repo: %w", src, err)
			}
			if _, err = w.Add(fName); err != nil {
				return fmt.Errorf("Error adding %q to manifest git index: %w", src, err)
			}
		}
	}
	src := filepath.Join(newdir, "manifest.yaml")
	dest := filepath.Join(mPath, "manifest.yaml")
	if err := CopyFileBits(src, dest); err != nil {
		return fmt.Errorf("Failed copying manifest to final directory")
	}
	if _, err = w.Add("manifest.yaml"); err != nil {
		return fmt.Errorf("Error adding manifest.yaml to manifest git index: %w", err)
	}

	commitOpts := &git.CommitOptions{
		Author:    defaultSignature(),
		Committer: defaultSignature(),
	}
	msg := "System upgrade to (TODO - fill in version)" // TODO
	if _, err := w.Commit(msg, commitOpts); err != nil {
		return fmt.Errorf("Failed committing to git")
	}

	return nil
}

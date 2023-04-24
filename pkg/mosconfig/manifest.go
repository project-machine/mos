package mosconfig

import (
	"encoding/json"
	"fmt"
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
)

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

	is := InstallSource{
		FilePath: manifestPath,
		CertPath: manifestCert,
		SignPath: manifestPath + ".signed",
	}
	cf, err := ReadVerifyInstallManifest(is, manifestCA, mos.storage)
	if err != nil {
		return fmt.Errorf("Failed verifying signature on %s: %w", manifestPath, err)
	}

	sFile := fmt.Sprintf("%s.json.signed", shaSum)
	dest := filepath.Join(dir, sFile)
	err = CopyFileBits(manifestPath+".signed", dest)
	if err != nil {
		return fmt.Errorf("Failed copying install manifest: %w", err)
	}
	_, err = w.Add(sFile)
	if err != nil {
		return fmt.Errorf("Git file add for manifest signature (%q) failed: %w", sFile, err)
	}

	mFile := fmt.Sprintf("%s.json", shaSum)
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

	dest = filepath.Join(dir, "manifest.json")
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

	bytes, err := json.Marshal(&sysmanifest)
	if err != nil {
		return fmt.Errorf("Failed marshalling the system manifest")
	}

	err = os.WriteFile(dest, bytes, 0640)
	if err != nil {
		return fmt.Errorf("Failed writing system manifest: %w", err)
	}
	_, err = w.Add("manifest.json")
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
	ociDir := filepath.Join(mos.opts.StorageCache, "mos")
	oci, err := umoci.OpenLayout(ociDir)
	if err != nil {
		return emptyM, emptyC, errors.Wrapf(err, "Failed reading OCI manifest for %s", t.ServiceName)
	}
	defer oci.Close()

	ociManifest, err := stackeroci.LookupManifest(oci, t.Digest)
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

	contents, err := os.ReadFile(filepath.Join(clonedir, "manifest.json"))
	if err != nil {
		return nil, fmt.Errorf("Error opening manifest: %w", err)
	}

	var sysmanifest SysManifest
	err = json.Unmarshal(contents, &sysmanifest)
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

	pemName := strings.TrimSuffix(yName, ".json") + ".pem"

	tmpd, err := os.MkdirTemp("", "verify")
	if err != nil {
		return InstallFile{}, fmt.Errorf("Failed creating a tempdir: %w", err)
	}
	defer os.RemoveAll(tmpd)

	is := InstallSource{
		FilePath:     filepath.Join(gitdir, yName),
		CertPath:     filepath.Join(gitdir, pemName),
		SignPath:     filepath.Join(gitdir, fmt.Sprintf("%s.signed", yName)),
		NeedsCleanup: false,
	}
	manifest, err := ReadVerifyInstallManifest(is, mos.opts.CaPath, mos.storage)
	if err != nil {
		return InstallFile{}, errors.Wrapf(err, "Failed verifying signature for target %q", yName)
	}

	l[yName] = manifest
	return manifest, nil
}

func (mos *Mos) UpdateManifest(manifest *SysManifest, newmanifest *SysManifest, newdir string) error {
	// Check out a new branch, copy over each required install.json
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

	// Copy any needed source jsons into our tempdir
	for _, t := range manifest.SysTargets {
		f := t.Source
		if PathExists(filepath.Join(newdir, f)) {
			continue
		}
		base := strings.TrimSuffix(f, ".json")
		for _, ext := range []string{".json", ".json.signed", ".pem"} {
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
		base := strings.TrimSuffix(f, ".json")
		for _, ext := range []string{".json", ".json.signed", ".pem"} {
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
	src := filepath.Join(newdir, "manifest.json")
	dest := filepath.Join(mPath, "manifest.json")
	if err := CopyFileBits(src, dest); err != nil {
		return fmt.Errorf("Failed copying manifest to final directory")
	}
	if _, err = w.Add("manifest.json"); err != nil {
		return fmt.Errorf("Error adding manifest.json to manifest git index: %w", err)
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

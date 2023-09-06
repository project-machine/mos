package mosconfig

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/apex/log"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
	"stackerbuild.io/stacker/pkg/lib"
)

type BootMode int

const (
	PathESPImage  = "loader/images/efi-esp.img"
	Bios          = "bios"
	BootLayerName = "live-boot:latest"
	ISOLabel      = "OCI-BOOT"
	ImplDiskfs    = "diskfs"
)

const (
	EFIAuto BootMode = iota
	EFIShim
	EFIKernel
)
const SBATContent = `sbat,1,SBAT Version,sbat,1,https://github.com/rhboot/shim/blob/main/SBAT.md
stubby.puzzleos,2,PuzzleOS,stubby,1,https://github.com/puzzleos/stubby
linux.puzzleos,1,PuzzleOS,linux,1,NOURL
`

const fat32BlockSize = 512

var EFIBootModeStrings = map[string]BootMode{
	"efi-auto":   EFIAuto,
	"efi-shim":   EFIShim,
	"efi-kernel": EFIKernel,
}

var EFIBootModes = map[BootMode]string{
	EFIAuto:   "efi-auto",
	EFIShim:   "efi-shim",
	EFIKernel: "efi-kernel",
}

type ISOOptions struct {
	EFIBootMode BootMode
	CommandLine string
}

type DiskOptions struct {
	EFIBootMode BootMode
	CommandLine string
	Size        int64
	Impl        string
}

func (opts ISOOptions) Check() error {
	if _, ok := EFIBootModes[opts.EFIBootMode]; !ok {
		return fmt.Errorf("Invalid boot mode %d", opts.EFIBootMode)
	}
	return nil
}

func (opts ISOOptions) MkisofsArgs() ([]string, error) {
	return []string{"-eltorito-alt-boot", "-e", PathESPImage, "-no-emul-boot", "-isohybrid-gpt-basdat"}, nil
}

const layoutTree, layoutFlat, layoutNone = "tree", "flat", ""

type OciBoot struct {
	KeySet         string            // the trust keyset (e.g. snakeoil) to use
	Project        string            // project within @KeySet whose keys to sign with
	BootURL        string            // the docker:// or oci: URL to the manifest to boot
	BootStyle      string            // Uh, not actually sure - shim or kernel?
	Files          map[string]string // file to copy in
	Cdrom          bool              // if true, build a cdrom
	Cmdline        string            // The extra kernel command line to pass
	BootKit        string            // path to directory with bootkit artifacts
	ZotPort        int               // port on which a local zot is running
	OutFile        string            // The output file (iso or qcow)
	BootFromRemote bool              // if true, manifest and oci layers are not copied onto boot media
	RepoDir        string            // The directory against which zot is running - to optionally rsync into iso
}

func (o *OciBoot) getBootKit() error {
	trustDir, err := MosKeyPath()
	if err != nil {
		return err
	}
	keysetPath := filepath.Join(trustDir, o.KeySet)
	if !PathExists(keysetPath) {
		return fmt.Errorf("Keyset not found: %s", o.KeySet)
	}

	o.BootKit = filepath.Join(keysetPath, "bootkit")

	return nil
}

// PopulateEFI - populate destd with files for an efi tree.
//
//	destd will have efi/ under it.
func (o *OciBoot) PopulateEFI(mode BootMode, cmdline string, destd string) error {
	const EFIBootDir = "/efi/boot/"
	const StartupNSHPath = "startup.nsh"
	const KernelEFI = "kernel.efi"
	const ShimEFI = "shim.efi"
	const mib = 1024 * 1024

	if mode == EFIAuto {
		mode = EFIKernel
		if PathExists(filepath.Join(o.BootKit, "shim.efi")) {
			mode = EFIShim
		}
	}

	fullCmdline := ""
	if o.BootURL != "" {
		// FIXME: fullCmdline root= should be based on type of o.BootLayer (root=soci or root=oci)
		n := o.BootURL
		repo := ""
		if strings.HasPrefix(n, "docker://") {
			n = strings.TrimPrefix(n, "docker://")
			split := strings.SplitN(n, "/", 2)
			if len(split) != 2 {
				return fmt.Errorf("Bad boot URL: %s", o.BootURL)
			}
			if o.BootFromRemote {
				repo = split[0]
			} else {
				repo = "local"
			}
			n = split[1]
		} else if strings.HasPrefix(n, "oci:") {
			repo = ""
			split := strings.SplitN(n, ":", 3)
			if len(split) != 3 {
				return fmt.Errorf("bad oci url: %s", o.BootURL)
			}
			n = split[2]
		} else {
			return fmt.Errorf("Unknown boot url: %s", o.BootURL)
		}

		fullCmdline = "root=soci:name=" + n + ",dev=LABEL=" + ISOLabel
		if repo != "" {
			fullCmdline += ",repo=" + repo
		}
	}
	if cmdline != "" {
		fullCmdline = fullCmdline + " " + cmdline
	}

	// should get the total size of all the source files and compute this.
	var startupNshContent = []string{
		"fs0:",
		"cd fs0:" + EFIBootDir,
	}

	copies := map[string]string{}
	if mode == EFIShim {
		copies[filepath.Join(o.BootKit, "shim.efi")] = EFIBootDir + ShimEFI
		copies[filepath.Join(o.BootKit, "kernel.efi")] = EFIBootDir + KernelEFI
		startupNshContent = append(startupNshContent, ShimEFI+" "+KernelEFI+" "+fullCmdline)
	} else if mode == EFIKernel {
		copies[filepath.Join(o.BootKit, "kernel.efi")] = KernelEFI
		startupNshContent = append(startupNshContent, KernelEFI+" "+fullCmdline)
	}

	startupNshContent = append(startupNshContent, "")

	if err := os.MkdirAll(filepath.Join(destd, EFIBootDir), 0755); err != nil {
		return err
	}

	// need to write a startup.nsh here
	efiboot := filepath.Join(destd, EFIBootDir)
	if err := os.WriteFile(filepath.Join(efiboot, StartupNSHPath),
		[]byte(strings.Join(startupNshContent, "\n")), 0644); err != nil {
		return err
	}

	for src, dst := range copies {
		if err := copyFile(src, filepath.Join(destd, dst)); err != nil {
			return err
		}
	}

	return nil
}

// return total of all files under path
func getDirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return err
	})
	return size, err
}

func (o *OciBoot) genESP(opts ISOOptions, fname string) error {

	tmpd, err := ioutil.TempDir("", "genESP-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpd)
	if err := o.PopulateEFI(opts.EFIBootMode, opts.CommandLine, tmpd); err != nil {
		return err
	}
	if err := genESP(fname, tmpd); err != nil {
		return err
	}

	return nil
}

// baseDir is expected to have efi/ in it.
func genESP(fname string, baseDir string) error {
	treeSize, err := getDirSize(baseDir)
	if err != nil {
		return err
	}
	// assumed fat filesystem overhead
	const overhead = 1.05
	size := int64(float64(treeSize) * overhead)

	fp, err := os.OpenFile(fname, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return fmt.Errorf("Failed to open %s for Create: %w", fname, err)
	}
	if err := unix.Ftruncate(int(fp.Fd()), size); err != nil {
		log.Fatalf("Truncate '%s' failed: %s", fname, err)
	}
	if err := fp.Close(); err != nil {
		return fmt.Errorf("Failed to close file %s", fname)
	}

	if err := RunCommand("mkfs.fat", "-s", "1", "-F", "32", "-n", "EFIBOOT", fname); err != nil {
		return fmt.Errorf("mkfs.fat failed: %w", err)
	}

	cmd := []string{"env", "MTOOLS_SKIP_CHECK=1", "mcopy", "-s", "-v", "-i", fname,
		filepath.Join(baseDir, "efi"), "::efi"}
	log.Debugf("Running: %s", strings.Join(cmd, " "))
	if err := RunCommand(cmd...); err != nil {
		return err
	}
	return nil
}

func copyFile(src, dest string) error {
	fin, err := os.Open(src)
	if err != nil {
		return err
	}

	info, err := fin.Stat()
	if err != nil {
		return err
	}

	defer fin.Close()

	fout, err := os.OpenFile(dest, os.O_RDWR|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer fout.Close()

	_, err = io.Copy(fout, fin)

	if err != nil {
		return err
	}

	return nil
}

func (o *OciBoot) Build() error {
	if err := o.getBootKit(); err != nil {
		return err
	}

	repo, err := NewDistRepo(o.BootURL)
	if err != nil {
		return err
	}

	disturl, err := repo.openUrl(o.BootURL)
	if err != nil {
		return err
	}

	is := InstallSource{}
	if err := is.FetchFromZot(o.BootURL); err != nil {
		return errors.Wrapf(err, "Error fetching remote manifest %s", o.BootURL)
	}
	defer is.Cleanup()

	imgname := disturl.name + ":" + disturl.tag
	if err := is.SaveToZot(o.ZotPort, imgname); err != nil {
		return errors.Wrapf(err, "Failed saving image manifest to %s on local zot", imgname)
	}

	manifest, err := simpleParseInstall(is.FilePath)
	if err != nil {
		return errors.Wrapf(err, "Failed reading the install manifest")
	}

	for _, t := range manifest.Targets {
		//src := "docker://" + repo.addr + "/" + disturl.name + ":" + disturl.tag
		//dest := fmt.Sprintf("docker://127.0.0.1:%d/%s:%s", o.ZotPort, disturl.name, disturl.tag)
		src := fmt.Sprintf("docker://%s/mos:%s", repo.addr, dropHashPrefix(t.Digest))
		dest := fmt.Sprintf("docker://127.0.0.1:%d/mos:%s", o.ZotPort, dropHashPrefix(t.Digest))
		copyOpts := lib.ImageCopyOpts{
			Src:         src,
			Dest:        dest,
			Progress:    os.Stdout,
			SrcSkipTLS:  true,
			DestSkipTLS: true,
		}
		log.Debugf("Copying %s to %s using %#v", src, dest, copyOpts)
		if err := lib.ImageCopy(copyOpts); err != nil {
			return errors.Wrapf(err, "failed copying layer")
		}
	}

	if !o.Cdrom {
		return fmt.Errorf("non-iso image not yet implemented")
	}

	// TODO - write the rest of the gunk
	// create the actual image
	tmpd, err := ioutil.TempDir("", "OciBootCreate-")
	if err != nil {
		return err
	}

	defer os.RemoveAll(tmpd)

	mode := o.BootStyle
	efiMode := EFIAuto
	n, ok := EFIBootModeStrings[mode]
	if !ok {
		return fmt.Errorf("Unexpected --boot=%s. Expect one of: %v", mode, EFIBootModeStrings)
	}
	efiMode = n

	imgPath := filepath.Join(tmpd, PathESPImage)
	if err := os.MkdirAll(filepath.Dir(imgPath), 0755); err != nil {
		return fmt.Errorf("Could not make dir for %s in tmpdir: %v", PathESPImage, err)
	}
	opts := ISOOptions{
		EFIBootMode: efiMode,
		CommandLine: o.Cmdline,
	}
	if err := o.genESP(opts, imgPath); err != nil {
		return err
	}

	modSquashDest := filepath.Join(tmpd, "krd", "modules.squashfs")
	if err := os.MkdirAll(filepath.Dir(modSquashDest), 0755); err != nil {
		return fmt.Errorf("Failed to create directory for modules.squashfs: %v", err)
	}
	src := filepath.Join(o.BootKit, "kernel", "modules.squashfs")
	if !PathExists(src) {
		src = filepath.Join(o.BootKit, "modules.squashfs")
	}
	if err := copyFile(src, modSquashDest); err != nil {
		return fmt.Errorf("Failed to copy modules.squashfs to media: %v", err)
	}

	for src, dest := range o.Files {
		if err := copyFile(src, filepath.Join(tmpd, dest)); err != nil {
			return fmt.Errorf("Failed to copy file '%s' to iso path '%s': %w", src, dest, err)
		}
	}

	mkopts, err := opts.MkisofsArgs()
	if err != nil {
		return err
	}

	if !o.BootFromRemote {
		// copy the zot backing dir in
		// XXX TODO yikes, but will zot still be writing stuff out?
		src := o.RepoDir + "/"
		dest := filepath.Join(tmpd, "oci")
		if err := RunCommand("rsync", "-va", src, dest+"/"); err != nil {
			return errors.Wrapf(err, "Failed copying zot cache")
		}
	}

	cmd := []string{
		"xorriso",
		"-compliance", "iso_9660_level=3",
		"-as", "mkisofs",
		"-o", o.OutFile,
		"-V", ISOLabel,
	}

	cmd = append(cmd, mkopts...)
	cmd = append(cmd, tmpd)

	log.Infof("Executing: %s", strings.Join(cmd, " "))
	if err := RunCommand(cmd...); err != nil {
		return err
	}

	return nil
}

// A template for a provisioning ISO manifest.yaml.  It only needs to specify
// the layer which mosctl, during initrd, should mount as the RFS.
// I'd like to make service_name be 'provision', but then the initrd would
// need to be updated, as it currently always does:
//
//	set -- mosctl $debug mount \
//	    "--target=livecd" \
//	    "--dest=$rootd" \
//	    "${repo}/$name"
var manifestTemplate = `
version: 1
product: "%s"
update_type: complete
targets:
  - service_name: livecd
    source: "docker://zothub.io/machine/bootkit/provision-rootfs:%s-squashfs"
    version: %s
    service_type: fs-only
    nsgroup: "none"
    network:
      type: none
`

// Build a provisioning ISO for the given keyset
func BuildProvisioner(keysetName, projectName, isofile string) error {
	dir, err := os.MkdirTemp("", "provision")
	if err != nil {
		return errors.Wrapf(err, "failed creating temporary directory")
	}
	defer os.RemoveAll(dir)

	cacheDir := filepath.Join(dir, "cache")
	zotPort, cleanup, err := StartZot(dir, cacheDir)
	if err != nil {
		return errors.Wrapf(err, "failed starting a local zot")
	}
	defer cleanup()

	repo := fmt.Sprintf("127.0.0.1:%d", zotPort)
	name := "machine/livecd:1.0.0"

	keyPath, err := MosKeyPath()
	if err != nil {
		return errors.Wrapf(err, "Failed finding mos key path")
	}
	pPath := filepath.Join(keyPath, keysetName, "manifest", projectName, "uuid")
	productUUID, err := os.ReadFile(pPath)
	if err != nil {
		return errors.Wrapf(err, "Failed to read project uuid (%q)", pPath)
	}
	manifestText := fmt.Sprintf(manifestTemplate, string(productUUID), LayerVersion, LayerVersion)

	manifestpath := filepath.Join(dir, "manifest.yaml")
	err = os.WriteFile(manifestpath, []byte(manifestText), 0600)
	if err != nil {
		return errors.Wrapf(err, "failed writing the manifest file")
	}

	fullproject := keysetName + ":" + projectName
	err = PublishManifest(fullproject, repo, name, manifestpath)
	if err != nil {
		return errors.Wrapf(err, "Failed writing manifest artifacts to local zot")
	}

	bootUrl := "docker://" + repo + "/" + name
	cmdline := "console=ttyS0"
	o := OciBoot{
		KeySet:         keysetName,
		Project:        fullproject,
		BootURL:        bootUrl,
		BootStyle:      EFIBootModes[EFIAuto],
		OutFile:        isofile,
		Cdrom:          true,
		Cmdline:        cmdline,
		BootFromRemote: false,
		RepoDir:        cacheDir,
		Files:          map[string]string{},
		ZotPort:        zotPort,
	}

	return o.Build()
}

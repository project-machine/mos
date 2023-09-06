package main

import (
	"archive/tar"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/apex/log"
	"golang.org/x/sys/unix"

	"github.com/anuvu/disko"
	"github.com/anuvu/disko/linux"
	"github.com/anuvu/disko/partid"
	"github.com/diskfs/go-diskfs/filesystem/fat32"
	ispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opencontainers/umoci"
	"github.com/opencontainers/umoci/oci/casext"
	"github.com/opencontainers/umoci/oci/layer"
	cli "github.com/urfave/cli/v2"
	"stackerbuild.io/stacker/pkg/lib"
	stackeroci "stackerbuild.io/stacker/pkg/oci"
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

type ociPath struct {
	Repo   string
	Name   string
	Tag    string
	layout string
}

func (o ociPath) String() string {
	return "oci:" + o.OciDir() + ":" + o.RefName()
}

func (o *ociPath) OciDir() string {
	if o.layout == layoutTree {
		return filepath.Join(o.Repo, o.Name)
	}
	return o.Repo
}

// the name that you would look for in a manifest
func (o *ociPath) RefName() string {
	if o.layout == layoutTree {
		return o.Tag
	}
	if o.Name == "" {
		return o.Tag
	}
	return o.Name + ":" + o.Tag
}

// the name (namespace) and tag of this entry.
func (o *ociPath) NameAndTag() string {
	if o.Name == "" {
		return o.Tag
	}
	return o.Name + ":" + o.Tag
}

func newOciPath(ref string, layout string) (*ociPath, error) {
	// oci:dir:[name:]tag
	toks := strings.Split(ref, ":")
	num := len(toks)

	p := &ociPath{}
	if num <= 2 {
		return p, fmt.Errorf("Not enough ':' in '%s'. Need 2 or 3, found %d", ref, num-1)
	} else if num > 4 {
		return p, fmt.Errorf("Too many ':' in '%s'. Need 2 or 3, found %d", ref, num-1)
	}

	p.layout = layout
	p.Repo = toks[1]
	switch p.layout {
	case layoutTree, layoutFlat:
	case layoutNone:
		p.layout = layoutTree
		if PathExists(filepath.Join(p.Repo, "index.json")) {
			p.layout = layoutFlat
		}
	default:
		return p, fmt.Errorf("unknown layout %s", layout)
	}

	if num == 3 {
		p.Tag = toks[2]
	} else {
		p.Name = toks[2]
		p.Tag = toks[3]
	}

	return p, nil
}

func ociExtractRef(image, dest string) error {
	tmpOciDir, err := ioutil.TempDir("", "extractRef-")
	if err != nil {
		return err
	}
	const tmpName = "xxextract"
	defer os.RemoveAll(tmpOciDir)

	dp, err := newOciPath("oci:"+tmpOciDir+":"+tmpName, layoutTree)
	if err != nil {
		return err
	}

	if err := doCopy(image, dp.String()); err != nil {
		return fmt.Errorf("copy %s -> %s failed: %w", image, dp.String(), err)
	}

	log.Debugf("ok, that went well, now openLayout(%s)", tmpOciDir)
	ociDir := dp.OciDir()
	oci, err := umoci.OpenLayout(ociDir)
	if err != nil {
		return err
	}
	defer oci.Close()

	log.Debugf("ok, that went well, now unpack %s, %s, %s, %s", tmpOciDir, oci, tmpName, dest)

	return unpackLayerRootfs(ociDir, oci, dp.RefName(), dest)
}

func unpackLayerRootfs(ociDir string, oci casext.Engine, tag string, extractTo string) error {
	// UnpackLayer creates rootfs config.json, sha256_<hash>.mtree umoci.json
	// but we want just the contents of rootfs in extractTo
	rootless := syscall.Geteuid() != 0
	log.Infof("extracting %s -> %s (rootless=%v)", tag, extractTo, rootless)

	xdir := path.Join(extractTo, ".extract")
	rootfs := path.Join(xdir, "rootfs")
	defer os.RemoveAll(xdir)

	if err := UnpackLayer(ociDir, oci, tag, xdir, rootless); err != nil {
		return err
	}

	entries, err := ioutil.ReadDir(rootfs)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if err := os.Rename(path.Join(rootfs, entry.Name()), path.Join(extractTo, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func UnpackLayer(ociDir string, oci casext.Engine, tag string, dest string, rootless bool) error {
	manifest, err := stackeroci.LookupManifest(oci, tag)
	if err != nil {
		return fmt.Errorf("couldn't find '%s' in oci: %w", tag, err)
	}

	if manifest.Layers[0].MediaType == ispec.MediaTypeImageLayer ||
		manifest.Layers[0].MediaType == ispec.MediaTypeImageLayerGzip {
		os := layer.UnpackOptions{KeepDirlinks: true}
		if rootless {
			os.MapOptions, err = GetRootlessMapOptions()
			if err != nil {
				return err
			}
		}
		err = umoci.Unpack(oci, tag, dest, os)
		if err != nil {
			return err
		}
	} else {
		if err := unpackSquashLayer(ociDir, oci, tag, dest, rootless); err != nil {
			return err
		}
	}
	return nil
}

// after calling getBootkit, the returned path will have 'bootkit' under it.
func getBootKit(ref string) (func() error, string, error) {
	cleanup := func() error { return nil }
	path := ""
	var err error
	if strings.HasPrefix(ref, "oci:") || strings.HasPrefix(ref, "docker:") {
		var tmpd string
		tmpd, err = ioutil.TempDir("", "getBootKit-")
		if err != nil {
			return cleanup, tmpd, err
		}
		cleanup = func() error { return os.RemoveAll(tmpd) }
		path = tmpd
		if err = ociExtractRef(ref, path); err != nil {
			return cleanup, path, err
		}
	} else {
		// local dir existing.
		path, err = filepath.Abs(ref)
	}

	if PathExists(filepath.Join(path, "export")) {
		// drop a top level 'export'
		path = filepath.Join(path, "export")
	}

	required := []string{"bootkit"}
	errmsgs := []string{}

	for _, r := range required {
		// for each entry in required, accept an existing dir
		// if no dir, then extract the .tar with that name.
		dirPath := filepath.Join(path, r)
		if isDir(dirPath) {
			continue
		}
	}

	if len(errmsgs) != 0 {
		return cleanup, path, fmt.Errorf("bootkit at %s had errors:\n  %s\n", ref, strings.Join(errmsgs, "\n  "))
	}

	return cleanup, path, nil
}

func untar(tarball, target string) error {
	reader, err := os.Open(tarball)
	if err != nil {
		return err
	}

	if err != nil {
		return err
	}
	defer reader.Close()
	tarReader := tar.NewReader(reader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		path := filepath.Join(target, header.Name)
		info := header.FileInfo()
		if info.IsDir() {
			if err = os.MkdirAll(path, info.Mode()); err != nil {
				return err
			}
			continue
		}

		file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(file, tarReader)
		if err != nil {
			return err
		}
	}
	return nil
}

func isDir(fpath string) bool {
	file, err := os.Open(fpath)
	if err != nil {
		return false
	}
	fi, err := file.Stat()
	if err != nil {
		return false
	}
	return fi.IsDir()
}

type OciBoot struct {
	BootKit    string            `json:"bootkit"`
	BootLayer  string            `json:"boot-layer"`
	Files      map[string]string `json:"files"`
	Layers     []string          `json:"layers"`
	cleanups   []func() error
	bootKitDir string
	RepoDir    string `json:"repodir"`
}

func genGptDisk(fpath string, fsize int64) (disko.Disk, error) {
	disk := disko.Disk{
		Name:       "disk",
		Path:       fpath,
		Size:       uint64(fsize),
		SectorSize: 512,
		Table:      disko.GPT,
	}

	if err := ioutil.WriteFile(fpath, []byte{}, 0600); err != nil {
		return disk, fmt.Errorf("Failed to write to a temp file: %s", err)
	}

	if err := os.Truncate(fpath, fsize); err != nil {
		return disk, fmt.Errorf("Failed create empty file: %s", err)
	}

	fs := disk.FreeSpaces()
	if len(fs) != 1 {
		return disk, fmt.Errorf("Expected 1 free space, found %d", fs)
	}

	parts := disko.PartitionSet{
		1: disko.Partition{
			Start:  fs[0].Start,
			Last:   fs[0].Last,
			Type:   partid.EFI,
			Name:   "EFI",
			ID:     disko.GenGUID(),
			Number: uint(1),
		}}

	lSys := linux.System()
	if err := lSys.CreatePartitions(disk, parts); err != nil {
		return disk, err
	}

	disk.Partitions = parts

	return disk, nil
}

// This will probably only work if the filesystem has just been created.
func createAndCopyToFat32Mtools(srcDir string, diskFile string, fsStart int64, fsSize int64) error {
	// trim the trailing /

	const kb int64 = 1024
	const secSize int64 = 512
	size := fsSize / kb
	if fsSize%kb != 0 {
		return fmt.Errorf("Size '%d' is not multiple of %d", fsSize, kb)
	}

	args := []string{
		"mkfs.fat",
		"-F32",          // fat size - 32 for fat32 fs.
		"-n" + ISOLabel, // filesystem label
		fmt.Sprintf("--offset=%d", fsStart/secSize), // offset specified in sectors.
		diskFile,
		fmt.Sprintf("%d", size), // size in kb
	}
	log.Debugf("Formatting with %s", strings.Join(args, "  "))
	if err := RunCommand(args...); err != nil {
		return fmt.Errorf("Failed to create filesystem with %s: %v", strings.Join(args, " "), err)
	}

	fullPath, err := filepath.Abs(diskFile)
	if err != nil {
		return fmt.Errorf("Could not get full path to %s: %v", diskFile, err)
	}

	dirFp, err := os.Open(srcDir)
	if err != nil {
		return fmt.Errorf("Failed to open directory '%s': %v", srcDir, err)
	}
	files, err := dirFp.Readdirnames(0)
	if err != nil {
		dirFp.Close()
		return fmt.Errorf("Failed to read files in '%s': %v", srcDir, err)
	}
	dirFp.Close()

	args = []string{
		"env", "MTOOLS_SKIP_CHECK=1",
		"mcopy",
		"-s", // recursive
		"-v", // verbose
		// -i filename@@offset
		"-i", fmt.Sprintf("%s@@%d", fullPath, fsStart),
	}
	args = append(args, append(files, "::")...)
	log.Debugf("Running in %s: %s", srcDir, strings.Join(args, " "))

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = srcDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s: %s", strings.Join(args, " "), err, string(output))
	}
	return nil
}

func createAndCopyToFat32DiskFS(srcDir string, diskFile string, fsStart int64, fsSize int64) error {
	fp, err := os.OpenFile(diskFile, os.O_RDWR|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	defer fp.Close()
	fs, err := fat32.Create(fp, fsSize, fsStart, fat32BlockSize, ISOLabel)
	if err != nil {
		return fmt.Errorf("Failed to create fat32 fs in %s: %v", fp.Name(), err)
	}

	// trim the trailing /
	if strings.HasSuffix(srcDir, "/") {
		srcDir = srcDir[0 : len(srcDir)-1]
	}
	return filepath.Walk(srcDir,
		func(fname string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if fname == srcDir {
				return nil
			}
			// src is /<tmpdir>/<path>
			// dest gets /<path>
			dest := fname[len(srcDir):]

			if info.IsDir() {
				// Mkdir behaves like `mkdir -p`: creates the full tree and does return EEXIST if existing.
				if err := fs.Mkdir(filepath.Dir(dest)); err != nil {
					return err
				}
			} else if info.Mode().IsRegular() {
				srcFile, err := os.Open(fname)
				if err != nil {
					return fmt.Errorf("Failed to read file %s (dest=%s)", fname, dest)
				}
				defer srcFile.Close()

				// Above does an Mkdir for the directory entry before any files in it
				// But that does not seem sufficient (unable to open directory).
				// maybe it needs a 'sync' or something?
				dir := filepath.Dir(dest)
				if err := fs.Mkdir(dir); err != nil {
					return fmt.Errorf("Failed to create '%s' for '%s'", dir, dest)
				}
				destFile, err := fs.OpenFile(dest, os.O_CREATE|os.O_RDWR)
				if err != nil {
					return fmt.Errorf("Failed to open dest '%s': %v", dest, err)
				}
				defer destFile.Close()

				fmt.Printf("copying to %s\n", dest)
				if _, err := io.Copy(destFile, srcFile); err != nil {
					return fmt.Errorf("Failed to copy from %s -> %s: %v", srcFile.Name(), dest, err)
				}
			} else {
				return fmt.Errorf("%s is not dir or regular file\n", fname)
			}
			return err
		})
}

func (o *OciBoot) CreateDisk(diskFile string, opts DiskOptions) error {
	if err := o.getBootKit(); err != nil {
		return err
	}

	tmpd, err := ioutil.TempDir("", "OciBootCreate-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpd)

	if err := o.Populate(tmpd); err != nil {
		return err
	}

	if err := o.PopulateEFI(opts.EFIBootMode, opts.CommandLine, tmpd); err != nil {
		return err
	}

	if opts.Size == 0 {
		opts.Size = 1 * 1024 * 1024 * 1024
	}

	disk, err := genGptDisk(diskFile, opts.Size)
	if err != nil {
		return err
	}

	p := disk.Partitions[1]
	fsSize := int64(p.Size())
	fsStart := int64(p.Start)

	if opts.Impl == ImplDiskfs {
		if err := createAndCopyToFat32DiskFS(tmpd, diskFile, fsStart, fsSize); err != nil {
			return err
		}
	} else {
		if err := createAndCopyToFat32Mtools(tmpd, diskFile, fsStart, fsSize); err != nil {
			return err
		}
	}
	return err
}

// Create - create an iso in isoFile
func (o *OciBoot) Create(isoFile string, opts ISOOptions) error {
	if err := opts.Check(); err != nil {
		return err
	}
	if err := o.getBootKit(); err != nil {
		return err
	}
	tmpd, err := ioutil.TempDir("", "OciBootCreate-")
	if err != nil {
		return err
	}

	defer os.RemoveAll(tmpd)

	imgPath := filepath.Join(tmpd, PathESPImage)
	if err := os.MkdirAll(path.Dir(imgPath), 0755); err != nil {
		return fmt.Errorf("Could not make dir for %s in tmpdir: %v", PathESPImage, err)
	}
	if err := o.genESP(opts, imgPath); err != nil {
		return err
	}

	if err := o.Populate(tmpd); err != nil {
		return err
	}

	mkopts, err := opts.MkisofsArgs()
	if err != nil {
		return err
	}
	cmd := []string{
		"xorriso",
		"-compliance", "iso_9660_level=3",
		"-as", "mkisofs",
		"-o", isoFile,
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

func ensureDir(dir string) error {
	return os.MkdirAll(dir, 0755)
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
		if PathExists(filepath.Join(o.bootKitDir, "bootkit/shim.efi")) {
			mode = EFIShim
		}
	}

	fullCmdline := ""
	if o.BootLayer != "" {
		// FIXME: fullCmdline root= should be based on type of o.BootLayer (root=soci or root=oci)
		fullCmdline = "root=soci:name=" + BootLayerName + ",dev=LABEL=" + ISOLabel
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
		copies[filepath.Join(o.bootKitDir, "bootkit/shim.efi")] = EFIBootDir + ShimEFI
		copies[filepath.Join(o.bootKitDir, "bootkit/kernel.efi")] = EFIBootDir + KernelEFI
		startupNshContent = append(startupNshContent, ShimEFI+" "+KernelEFI+" "+fullCmdline)
	} else if mode == EFIKernel {
		copies[filepath.Join(o.bootKitDir, "bootkit/kernel.efi")] = KernelEFI
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

func getImageName(ref string) string {
	// should be like 'oci:<path>:<name> - where 'name' might have : in it.
	// this is really simplistic but I just want the 'basename' effect
	// of 'mkdir dest; cp /some/file dest/'
	toks := strings.SplitN(ref, ":", 3)
	return toks[2]
}

// copy oci image at src to dest
// for 'oci:' src or dest
// if src or dest is of form:
//
//	oci:dir:[name:]tag
//
// Then attempt to support 'zot' layout
// this should also wo
func doCopy(src, dest string) error {
	log.Debugf("copying %s -> %s", src, dest)
	dpSrc, err := newOciPath(src, layoutNone)
	if err != nil {
		return err
	}

	dpDest, err := newOciPath(dest, layoutTree)
	if err != nil {
		return err
	}

	log.Debugf("Copying %s -> %s", dpSrc, dpDest)
	if err := os.MkdirAll(dpDest.OciDir(), 0755); err != nil {
		return fmt.Errorf("Failed to create directory %s for %s", dpDest.OciDir(), dpDest)
	}
	/*
		if strings.HasPrefix(normDest, "oci:") {
			dir := strings.SplitN(normDest, ":", 3)[1]
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("Failed to create dir for %s: %w", dest, err)
			}
		}
	*/
	if err := lib.ImageCopy(lib.ImageCopyOpts{Src: dpSrc.String(), Dest: dpDest.String(), Progress: os.Stderr}); err != nil {
		return fmt.Errorf("Failed copy %s -> %s\n", dpSrc, dpDest)
	}
	return nil
}

// given ref like : oci:dir:[name:]tag
// return either zot-layout format (tag in dir/name/index.json):
//
//	oci:dir[/name]:tag
//
// or oci layout format (name:tag in dir/index.json)
//
//	oci:dir:[name:]tag
func adjustOCIRef(ref string) (string, error) {
	// ocit = "oci tree"
	if strings.HasPrefix(ref, "ocit:") {
		toks := strings.Split(ref, ":")
		num := len(toks)
		if num == 4 {
			return "oci:" + toks[1] + "/" + toks[2] + ":" + toks[3], nil
		}
		return "", fmt.Errorf("ocit ref %s has %d toks", ref, num)
	}
	if !strings.HasPrefix(ref, "oci:") {
		return ref, nil
	}

	var dirPath, name, tag string
	toks := strings.Split(ref, ":")
	num := len(toks)

	if num <= 2 {
		return "", fmt.Errorf("Not enough ':' in '%s'. Need 2 or 3, found %d", ref, num-1)
	} else if num == 3 {
		// "oci":dir:name -> turn this into oci:dir:name:latest
		// which is what you'd get if you uploaded such a thing to zot
		dirPath = toks[1]
		name = toks[2]
	} else if num == 4 {
		dirPath = toks[1]
		name = toks[2]
		tag = toks[3]
	} else {
		return "", fmt.Errorf("Too many ':' in '%s'. Need 2 or 3, found %d", ref, num-1)
	}

	dirHasIndex := PathExists(filepath.Join(dirPath, "index.json"))
	log.Debugf("dirPath %s : %t", dirPath, dirHasIndex)

	if dirHasIndex {
		// existing oci repo
		if num == 3 {
			return "oci:" + dirPath + ":" + name, nil
		}
		// single oci dir layout.
		log.Debugf("ocilayout: %s -> %s", ref, "oci:"+dirPath+":"+name+":"+tag)
		return "oci:" + dirPath + ":" + name + ":" + tag, nil
	}

	// zot layout
	log.Debugf("zotlayout: %s -> %s", ref, "oci:"+dirPath+"/"+name+":"+tag)
	return "oci:" + dirPath + "/" + name + ":" + tag, nil
}

// populate the directory with the contents of the iso.
func (o *OciBoot) Populate(target string) error {
	ociDir := filepath.Join(target, "oci")
	if o.BootLayer != "" {
		log.Infof("Copying BootLayer %s -> %s:%s", o.BootLayer, ociDir, BootLayerName)
		dest := "oci:" + ociDir + ":" + BootLayerName
		if err := doCopy(o.BootLayer, dest); err != nil {
			return fmt.Errorf("Failed to copy image from BootLayer '%s': %w", o.BootLayer, err)
		}
	}

	log.Infof("ok, copied the bootlayer name")

	repoDir := filepath.Join(target, "zot-cache")
	if o.RepoDir != "" {
		src := o.RepoDir + "/"
		dest := repoDir + "/"
		args := []string{"rsync", "-va", src, dest}
		if err := RunCommand(args...); err != nil {
			return fmt.Errorf("Failed syncing %s/ -> %s: %w", src, dest, err)
		}
	}

	if len(o.Layers) != 0 {
		ociDest := "oci:" + ociDir + ":"
		for i, src := range o.Layers {
			dSrc, err := newOciPath(src, layoutNone)
			if err != nil {
				return err
			}
			dest := ociDest + dSrc.NameAndTag()
			log.Infof("Copying Layer %d/%d: %s -> %s", i+1, len(o.Layers), src, dest)
			if err := doCopy(src, dest); err != nil {
				return fmt.Errorf("Failed to copy %s -> %s: %w", src, dest, err)
			}
		}
	}

	modSquashDest := path.Join(target, "krd", "modules.squashfs")
	if err := os.MkdirAll(filepath.Dir(modSquashDest), 0755); err != nil {
		return fmt.Errorf("Failed to create directory for modules.squashfs: %v", err)
	}
	if err := copyFile(filepath.Join(o.bootKitDir, "bootkit/modules.squashfs"), modSquashDest); err != nil {
		return fmt.Errorf("Failed to copy modules.squashfs to media: %v", err)
	}

	for src, dest := range o.Files {
		if err := copyFile(src, path.Join(target, dest)); err != nil {
			return fmt.Errorf("Failed to copy file '%s' to iso path '%s': %w", src, dest, err)
		}
	}

	return nil
}

func writeTemp(content []byte) (string, error) {
	fh, err := os.CreateTemp("", "writeTemp")
	if err != nil {
		return "", err
	}
	if _, err := fh.Write(content); err != nil {
		os.Remove(fh.Name())
		return "", err
	}
	if err := fh.Close(); err != nil {
		os.Remove(fh.Name())
		return "", err
	}
	return fh.Name(), nil
}

func (o *OciBoot) Cleanup() error {
	for _, c := range o.cleanups {
		if err := c(); err != nil {
			return err
		}
	}
	return nil
}

func (o *OciBoot) getBootKit() error {
	if o.bootKitDir != "" {
		return nil
	}
	cleanup, path, err := getBootKit(o.BootKit)
	o.cleanups = append(o.cleanups, cleanup)
	if err != nil {
		return err
	}
	o.bootKitDir = path
	return nil
}

func doMain(ctx *cli.Context) error {
	if ctx.Bool("debug") {
		log.SetLevel(log.DebugLevel)
	}
	args := ctx.Args()
	if args.Len() < 2 {
		return fmt.Errorf("Need at very least 2 args: output, bootkit-source")
	}
	ociBoot := OciBoot{}

	output := args.Get(0)
	ociBoot.BootKit = args.Get(1)

	// TODO - we should probably instead just accept the distribution spec
	// url for the manifest, and ourselves copy the manifest, any needed
	// layers, and the referring artifacts.  For now just rsync the backing
	// directories.
	if ctx.IsSet("sync-repodir") {
		ociBoot.RepoDir = ctx.String("sync-repodir")
	}

	if args.Len() > 2 {
		ociBoot.BootLayer = args.Get(2)
	}

	if args.Len() > 3 {
		ociBoot.Layers = args.Slice()[3:]
	}

	ociBoot.Files = map[string]string{}
	for _, p := range ctx.StringSlice("insert") {
		toks := strings.SplitN(p, ":", 2)
		if len(toks) != 2 {
			return fmt.Errorf("--insert arg had no 'dest' (src:dest): %s", p)
		}
		ociBoot.Files[toks[0]] = toks[1]
	}

	mode := ctx.String("boot")
	efiMode := EFIAuto
	n, ok := EFIBootModeStrings[mode]
	if !ok {
		return fmt.Errorf("Unexpected --boot=%s. Expect one of: %v", mode, EFIBootModeStrings)
	}
	efiMode = n

	defer ociBoot.Cleanup()

	if ctx.Bool("cdrom") {
		opts := ISOOptions{
			EFIBootMode: efiMode,
			CommandLine: ctx.String("cmdline"),
		}

		if err := ociBoot.Create(output, opts); err != nil {
			return err
		}
	} else {
		opts := DiskOptions{
			EFIBootMode: efiMode,
			CommandLine: ctx.String("cmdline"),
		}
		if ctx.Bool("use-diskfs") {
			opts.Impl = ImplDiskfs
		}
		if err := ociBoot.CreateDisk(output, opts); err != nil {
			return err
		}
	}

	log.Infof("Wrote iso %s.", output)
	return nil
}

func main() {

	app := cli.NewApp()
	app.Name = "oci-boot"
	app.Usage = "create disk or iso to boot an oci layer: bootkit boot-layer oci-layers"
	app.Version = "1.0.1"
	app.Action = doMain
	app.Flags = []cli.Flag{
		&cli.BoolFlag{
			Name:  "debug",
			Usage: "display additional debug information on stderr",
		},
		&cli.BoolFlag{
			Name:  "cdrom",
			Usage: "create a cdrom (iso9660) rather than a disk",
		},
		&cli.StringFlag{
			Name:   "use-diskfs",
			Usage:  "use the go-diskfs for fat filesystem operations",
			Hidden: true,
		},
		&cli.StringFlag{
			Name:  "boot",
			Usage: "boot-mode: one of 'efi-shim', 'efi-kernel', or 'efi-auto'",
			Value: EFIBootModes[EFIAuto],
		},
		&cli.StringFlag{
			Name:  "cmdline",
			Usage: "cmdline: additional parameters for kernel command line",
		},
		&cli.StringSliceFlag{
			Name:  "sync-repodir",
			Usage: "Synchronize given repo directory to /zot-cache",
		},
		&cli.StringSliceFlag{
			Name:  "insert",
			Usage: "list of additional files in <src>:<dest> format to copy to iso",
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatalf("%v\n", err)
	}
}

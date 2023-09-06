package trust

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/apex/log"
	efi "github.com/canonical/go-efilib"
	ispec "github.com/opencontainers/image-spec/specs-go/v1"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/umoci"
	"github.com/opencontainers/umoci/oci/casext"
	"github.com/opencontainers/umoci/oci/layer"
	"github.com/pkg/errors"
	"github.com/project-machine/bootkit/pkg/cert"
	"github.com/project-machine/bootkit/pkg/shim"
	"github.com/project-stacker/stacker/container/idmap"
	"github.com/project-stacker/stacker/lib"
	stackeroci "github.com/project-stacker/stacker/oci"
)

func extractSingleSquash(squashFile string, extractDir string) error {
	err := os.MkdirAll(extractDir, 0755)
	if err != nil {
		return err
	}

	cmd := []string{"unsquashfs", "-f", "-d", extractDir, squashFile}
	return RunCommand(cmd...)
}

func unpackSquashLayer(ociDir string, oci casext.Engine, tag string, dest string) error {
	rootfsDir := filepath.Join(dest, "rootfs")
	manifest, err := stackeroci.LookupManifest(oci, tag)
	if err != nil {
		return errors.Wrapf(err, "Failed finding %s in oci layout", tag)
	}

	for _, layer := range manifest.Layers {
		squashFile := filepath.Join(ociDir, "blobs/sha256", layer.Digest.Encoded())
		if err := extractSingleSquash(squashFile, rootfsDir); err != nil {
			return errors.Wrapf(err, "Failed extracting squashfs")
		}
	}

	return nil
}

func GetRootlessMapOptions() (layer.MapOptions, error) {
	opts := layer.MapOptions{Rootless: true}
	idmapSet, err := idmap.ResolveCurrentIdmapSet()
	if err != nil {
		return opts, err
	}

	if idmapSet == nil {
		return opts, errors.Errorf("no uids mapped for current user")
	}

	for _, idm := range idmapSet.Idmap {
		if err := idm.Usable(); err != nil {
			return opts, errors.Errorf("idmap unusable: %s", err)
		}

		if idm.Isuid {
			opts.UIDMappings = append(opts.UIDMappings, rspec.LinuxIDMapping{
				ContainerID: uint32(idm.Nsid),
				HostID:      uint32(idm.Hostid),
				Size:        uint32(idm.Maprange),
			})
		}

		if idm.Isgid {
			opts.GIDMappings = append(opts.GIDMappings, rspec.LinuxIDMapping{
				ContainerID: uint32(idm.Nsid),
				HostID:      uint32(idm.Hostid),
				Size:        uint32(idm.Maprange),
			})
		}
	}

	return opts, nil
}

func UnpackLayer(ociDir string, oci casext.Engine, tag string, dest string) error {
	manifest, err := stackeroci.LookupManifest(oci, tag)
	if err != nil {
		return errors.Wrapf(err, "couldn't find '%s' in oci", tag)
	}

	if manifest.Layers[0].MediaType == ispec.MediaTypeImageLayer ||
		manifest.Layers[0].MediaType == ispec.MediaTypeImageLayerGzip {
		os := layer.UnpackOptions{KeepDirlinks: true}
		os.MapOptions, err = GetRootlessMapOptions()
		if err != nil {
			return errors.Wrapf(err, "Failed getting rootless map options")
		}
		err = umoci.Unpack(oci, tag, dest, os)
		if err != nil {
			return errors.Wrapf(err, "Failed unpacking layer")
		}
	} else {
		if err := unpackSquashLayer(ociDir, oci, tag, dest); err != nil {
			return errors.Wrapf(err, "Failed unpacking squashfs")
		}
	}
	return nil
}

func unpackLayerRootfs(ociDir string, oci casext.Engine, tag string, extractTo string) error {
	xdir := filepath.Join(extractTo, ".extract")
	rootfs := filepath.Join(xdir, "rootfs")
	defer os.RemoveAll(xdir)

	if err := UnpackLayer(ociDir, oci, tag, xdir); err != nil {
		return errors.Wrapf(err, "Failed unpacking layer")
	}

	entries, err := os.ReadDir(rootfs)
	if err != nil {
		return errors.Wrapf(err, "failed reading directory entries")
	}

	for _, entry := range entries {
		if err := os.Rename(filepath.Join(rootfs, entry.Name()), filepath.Join(extractTo, entry.Name())); err != nil {
			return errors.Wrapf(err, "Failed moving contents to %s", extractTo)
		}
	}
	return nil
}

func UpdateShim(inShim, newShim, keysetPath string) error {
	sigdataList, err := cert.LoadSignatureDataDirs(
		filepath.Join(keysetPath, "uki-limited"),
		filepath.Join(keysetPath, "uki-production"),
		filepath.Join(keysetPath, "uki-tpm"),
	)
	if err != nil {
		return errors.Wrapf(err, "Failed LoadSignatureDataDirs")
	}

	// Note, we are not doing sbattach --remove since we now ship without a signature

	err = shim.SetVendorDB(inShim, cert.NewEFISignatureDatabase(sigdataList),
		cert.NewEFISignatureDatabase([]*efi.SignatureData{}))

	cmd := []string{"sbsign",
		"--key", filepath.Join(keysetPath, "uefi-db", "privkey.pem"),
		"--cert", filepath.Join(keysetPath, "uefi-db", "cert.pem"),
		"--output", newShim, inShim}
	err = RunCommand(cmd...)
	if err != nil {
		return errors.Wrapf(err, "failed re-signing shim")
	}

	return nil
}

// SetupBootkit: create a custom bootkit for a keyset.
// bootkitVersion specifies the version of bootkit to download from
// zothub.io.  mosctlPath is a custom mosctl to insert - this is for
// testing.  Normally "" will be passed, and the mosctl used by bootkit
// will remain in use.
func SetupBootkit(keysetName, bootkitVersion, mosctlPath string) error {
	// TODO - we have to fix this by
	// a. having bootkit generate arm64
	// b. changing the bootkit layer naming to reflect arch
	// c. using the bootkit api here instead of doing it ourselves
	// for now, we just do nothing on arm64
	if runtime.GOARCH != "amd64" {
		log.Warnf("Running on %q, so not building bootkit artifacts (only amd64 supported).", runtime.GOARCH)
		return nil
	}

	tmpdir, err := os.MkdirTemp("", "trust-bootkit")
	if err != nil {
		return errors.Wrapf(err, "Failed creating temporary directory")
	}
	defer os.RemoveAll(tmpdir)

	home, err := os.UserHomeDir()
	if err != nil {
		return errors.Wrapf(err, "couldn't find home dir")
	}
	ociDir := filepath.Join(home, ".cache", "machine", "trust", "bootkit", "oci")
	bootkitLayer := "bootkit:" + bootkitVersion + "-squashfs"
	EnsureDir(ociDir)
	cachedOci := fmt.Sprintf("oci:%s:%s", ociDir, bootkitLayer)
	err = lib.ImageCopy(lib.ImageCopyOpts{
		Src:      fmt.Sprintf("docker://zothub.io/machine/bootkit/%s", bootkitLayer),
		Dest:     cachedOci,
		Progress: os.Stdout,
	})
	if err != nil {
		return errors.Wrapf(err, "Failed copying pristine bootkit")
	}

	oci, err := umoci.OpenLayout(ociDir)
	if err != nil {
		return errors.Wrapf(err, "Failed opening layout %s", ociDir)
	}
	defer oci.Close()

	bDir := filepath.Join(tmpdir, "bootkit")
	err = unpackLayerRootfs(ociDir, oci, bootkitLayer, bDir)
	if err != nil {
		return errors.Wrapf(err, "Failed unpacking bootkit layer")
	}

	// Now we have a directory 'bootkit/bootkit' let's flatten that for convenience
	os.Rename(filepath.Join(bDir, "bootkit"), bDir+".tmp")
	os.RemoveAll(bDir)
	os.Rename(bDir+".tmp", bDir)
	mosKeyPath, err := getMosKeyPath()
	if err != nil {
		return errors.Wrapf(err, "Failed getting mos keypath")
	}

	keysetPath := filepath.Join(mosKeyPath, keysetName)
	destDir := filepath.Join(keysetPath, "bootkit")
	if err := EnsureDir(destDir); err != nil {
		return errors.Wrapf(err, "Failed creating directory %q", destDir)
	}

	unchanged := []string{"kernel/modules.squashfs", "ovmf/ovmf-code.fd"}
	for _, f := range unchanged {
		if err := CopyFile(filepath.Join(bDir, f), filepath.Join(destDir, f)); err != nil {
			return errors.Wrapf(err, "Failed copying %s into new bootkit from %s -> %s", f, bDir, destDir)
		}
	}

	err = UpdateShim(filepath.Join(bDir, "shim", "shim.efi"), filepath.Join(destDir, "shim.efi"), keysetPath)
	if err != nil {
		return errors.Wrapf(err, "Failed updating the shim")
	}

	// break apart kernel.efi to replace the manifestCA.pem
	newKernel, err := ReplaceManifestCert(bDir, keysetPath, mosctlPath)
	if err != nil {
		return errors.Wrapf(err, "Failed replacing manifest certificate")
	}
	cmd := []string{"sbsign",
		"--key", filepath.Join(keysetPath, "uki-production", "privkey.pem"),
		"--cert", filepath.Join(keysetPath, "uki-production", "cert.pem"),
		"--output", filepath.Join(destDir, "kernel.efi"),
		newKernel}
	err = RunCommand(cmd...)
	if err != nil {
		return errors.Wrapf(err, "failed re-signing shim")
	}

	// generate a new ovmf-vars.fd
	pkGuidBytes, err := os.ReadFile(filepath.Join(keysetPath, "uefi-pk", "guid"))
	if err != nil {
		return errors.Wrapf(err, "failed reading uefi-pk guid")
	}
	pkGuid := strings.TrimSpace(string(pkGuidBytes))
	kekGuidBytes, err := os.ReadFile(filepath.Join(keysetPath, "uefi-kek", "guid"))
	if err != nil {
		return errors.Wrapf(err, "failed reading uefi-kek guid")
	}
	kekGuid := strings.TrimSpace(string(kekGuidBytes))
	dbGuidBytes, err := os.ReadFile(filepath.Join(keysetPath, "uefi-db", "guid"))
	if err != nil {
		return errors.Wrapf(err, "failed reading uefi-db guid")
	}
	dbGuid := strings.TrimSpace(string(dbGuidBytes))

	outFile := filepath.Join(destDir, "ovmf-vars.fd")
	plainvars := filepath.Join(bDir, "ovmf", "ovmf-vars.fd")
	cmd = []string{
		"virt-fw-vars",
		"--input=" + plainvars,
		"--output=" + outFile,
		"--secure-boot", "--no-microsoft",
		"--set-pk", pkGuid, filepath.Join(keysetPath, "uefi-pk", "cert.pem"),
		"--add-kek", kekGuid, filepath.Join(keysetPath, "uefi-kek", "cert.pem"),
		"--add-db", dbGuid, filepath.Join(keysetPath, "uefi-db", "cert.pem"),
	}
	if err := RunCommand(cmd...); err != nil {
		return errors.Wrapf(err, "Failed creating new ovmf vars")
	}

	return nil
}

func findSection(lines []string, which string) (int64, int64, bool) {
	for _, l := range lines {
		if strings.Contains(l, which) {
			s := strings.Fields(l)
			if len(s) != 7 {
				return 0, 0, false
			}
			sz, err := strconv.ParseInt(s[2], 16, 64)
			if err != nil {
				return 0, 0, false
			}
			off, err := strconv.ParseInt(s[5], 16, 64)
			if err != nil {
				return 0, 0, false
			}
			return off, sz, true
		}
	}
	return 0, 0, false
}

func extractObj(objdump []string, dir string, piece string) error {
	outName := filepath.Join(dir, piece+".out")
	offset, size, found := findSection(objdump, piece)
	if !found {
		return fmt.Errorf("Symbol %s not found", piece)
	}
	objPath := filepath.Join(dir, "kernel.efi")
	// Yes we could do this all without shelling out...
	err := RunCommand("dd", "if="+objPath, "of="+outName,
		fmt.Sprintf("skip=%d", offset),
		fmt.Sprintf("count=%d", size),
		"iflag=skip_bytes,count_bytes")
	if err != nil {
		return err
	}
	return nil
}

func appendToFile(dest, src string) error {
	from, err := os.Open(src)
	if err != nil {
		return errors.Wrapf(err, "Failed to open %s", src)
	}
	defer from.Close()
	to, err := os.OpenFile(dest, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return errors.Wrapf(err, "Failed to open %q for writing", dest)
	}
	defer to.Close()

	if _, err = io.Copy(to, from); err != nil {
		return errors.Wrapf(err, "Failed to copy from %q to %q", src, dest)
	}
	return nil
}

// Given a tempdir with bootkit artifacts, update it for our keyset.  In
// initrd, add newcert as /manifestCA.pem.  Build
// a new kernel.efi and return that filename.  Note that the filename
// will always be ${dir}/newkernel.efi, but whatever.
func ReplaceManifestCert(dir, keysetPath, customMostctl string) (string, error) {
	newCert := filepath.Join(keysetPath, "manifest-ca", "cert.pem")

	pcr7Dir := filepath.Join(keysetPath, "pcr7data")
	if !PathExists(pcr7Dir) {
		return "", fmt.Errorf("No pcr7data found")
	}
	pcr7Cpio := pcr7Dir + ".cpio"
	if !PathExists(pcr7Cpio) {
		if err := NewCpio(pcr7Cpio, pcr7Dir); err != nil {
			return "", errors.Wrapf(err, "Failed creating pcr7 cpio for %s", filepath.Base(keysetPath))
		}
	}

	initrd := filepath.Join(dir, "initrd.new")
	initrdgz := initrd + ".gz"
	certCpio := filepath.Join(dir, "newcert.initrd")

	emptydir, err := os.MkdirTemp("", "trust-cpio")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(emptydir)

	manifestCA := filepath.Join(emptydir, "manifestCA.pem")
	if err := CopyFile(newCert, manifestCA); err != nil {
		return "", errors.Wrapf(err, "Failed copying manifest into empty dir")
	}

	if err := NewCpio(certCpio, manifestCA); err != nil {
		return "", errors.Wrapf(err, "Failed creating cpio archive of manifest cert")
	}

	// Collect the pieces (bootkit api should do this for us)
	files := []string{
		filepath.Join(dir, "initrd/firmware.cpio.gz"),
		filepath.Join(dir, "initrd/core.cpio.gz"),
		filepath.Join(dir, "kernel/initrd-modules.cpio.gz"),
		filepath.Join(dir, "mos/initrd-mos.cpio.gz"),
		pcr7Cpio,
		certCpio,
	}
	if customMostctl != "" {
		mosctlDir := filepath.Join(emptydir, "/usr/bin")
		EnsureDir(mosctlDir)
		mosctlFile := filepath.Join(mosctlDir, "mosctl")
		if err := CopyFile(customMostctl, mosctlFile); err != nil {
			return "", errors.Wrapf(err, "Failed copying custom mosctl")
		}
		mosCpio := filepath.Join(dir, "mosctl.cpio")
		if err := NewCpio(mosCpio, filepath.Join(emptydir, "usr")); err != nil {
			return "", errors.Wrapf(err, "Failed creating mosctl cpio")
		}
		files = append(files, mosCpio)
		log.Infof("Inserting a custom mosctl into the initrd")
	}

	for _, f := range files {
		if strings.HasSuffix(f, ".gz") {
			if err := RunCommand("gunzip", f); err != nil {
				return "", errors.Wrapf(err, "Failed gunzipping %s", f)
			}
		}
		f = strings.TrimSuffix(f, ".gz")
		if err := appendToFile(initrd, f); err != nil {
			return "", errors.Wrapf(err, "Failed appending %s to initrd", f)
		}
	}

	if err := RunCommand("gzip", initrd); err != nil {
		return "", errors.Wrapf(err, "Failed re-zipping initrd.gz")
	}

	// Now build a kernel.efi

	kret := filepath.Join(dir, "newkernel.efi")
	cmd := []string{
		"objcopy",
		"--add-section=.cmdline=/dev/null",
		"--change-section-vma=.cmdline=0x30000",
		"--add-section=.sbat=" + filepath.Join(dir, "stubby/sbat.csv"),
		"--change-section-vma=.sbat=0x50000",
		"--set-section-alignment=.sbat=512",
		"--add-section=.linux=" + filepath.Join(dir, "kernel/boot/vmlinuz"),
		"--change-section-vma=.linux=0x1000000",
		"--add-section=.initrd=" + initrdgz,
		"--change-section-vma=.initrd=0x3000000",
		filepath.Join(dir, "stubby/stubby.efi"),
		kret,
	}
	if err := RunCommand(cmd...); err != nil {
		return "", errors.Wrapf(err, "Failed creating kernel.efi")
	}

	return kret, nil
}

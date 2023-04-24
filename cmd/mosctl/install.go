package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/anuvu/disko"
	"github.com/anuvu/disko/linux"
	"github.com/anuvu/disko/partid"
	"github.com/apex/log"
	"github.com/pkg/errors"
	"github.com/project-machine/mos/pkg/mosconfig"
	"github.com/project-machine/trust/pkg/trust"
	"github.com/urfave/cli"
)

var installCmd = cli.Command{
	Name:   "install",
	Usage:  "install a new mos system",
	Action: doInstall,
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "partition",
			Usage: "Wipe disks and create partitions for the new mos install\nIf not enabled, then use existing mounted filesystems",
		},
		cli.StringFlag{
			Name:  "root, rfs, r",
			Usage: "Directory under which to find the mos install",
			Value: "/",
		},
	},
}

type ptSizes map[string]uint64

const mib, gib = disko.Mebibyte, disko.Mebibyte * 1024
const gb = uint64(1000 * 1000 * 1000)
const espPart, configPart, storePart, scratchPart = "esp", "machine-config", "machine-store", "machine-scratch"

// PartitionTypeIDMachineConfig - F79962B9-24E6-9948-9F94-E6BFDAD2771A
var PartitionTypeIDMachineConfig = [16]byte{
	0xb9, 0x62, 0x99, 0xf7, 0xe6, 0x24, 0x48, 0x99, 0x9f, 0x94, 0xe6, 0xbf, 0xda, 0xd2, 0x77, 0x1a}

// PartitionTypeIDMachineStore - F79962B9-24E6-9948-9F94-E6BFDAD2771B
var PartitionTypeIDMachineStore = [16]byte{
	0xb9, 0x62, 0x99, 0xf7, 0xe6, 0x24, 0x48, 0x99, 0x9f, 0x94, 0xe6, 0xbf, 0xda, 0xd2, 0x77, 0x1b}

// PartitionTypeIDMachineScratch - F79962B9-24E6-9948-9F94-E6BFDAD2771C
var PartitionTypeIDMachineScratch = [16]byte{
	0xb9, 0x62, 0x99, 0xf7, 0xe6, 0x24, 0x48, 0x99, 0x9f, 0x94, 0xe6, 0xbf, 0xda, 0xd2, 0x77, 0x1c}

const minDiskSpace = 110 * gib

var errNoFactoryFound = errors.New("No Factory partitions found")

type newPart struct {
	Size uint64
	Name string
	Type disko.PartType
	ID   disko.GUID
}

// placePartitions - returns a Disk with the provided newParts placed or error.
func placePartitions(disk disko.Disk, parts []newPart) (disko.Disk, error) {
	const align = 1 * mib
	const minStart = 4 * mib

	var fslist []disko.FreeSpace

	if disk.Partitions == nil {
		disk.Partitions = disko.PartitionSet{}
	}
	for n, p := range parts {
		if p.Size == 0 {
			continue
		}
		evenSize := ""
		if p.Size%mib != 0 {
			evenSize = "!"
		}
		msg := fmt.Sprintf("part %d. size=%dMiB%s name=%s type=%s", n, p.Size/mib,
			evenSize, p.Name, p.Type)
		fslist = disk.FreeSpacesWithMin(p.Size)
		if len(fslist) == 0 {
			return disko.Disk{}, fmt.Errorf("No freespace: %s", msg)
		}
		num := uint(0)
		for i := uint(1); i <= 128; i++ {
			if _, ok := disk.Partitions[i]; !ok {
				num = i
				break
			}
		}
		if num == 0 {
			return disko.Disk{}, fmt.Errorf("No free numbers: %s", msg)
		}

		freespace := fslist[0]
		start := freespace.Start
		if freespace.Start < minStart {
			// only put bbp in the space < minStart. adjust starting pos of others.
			start = minStart
			shrunk := start - freespace.Start
			if freespace.Size()-shrunk < p.Size {
				// moving the start meant that this request wont fit here.
				if len(fslist) == 1 {
					return disko.Disk{},
						fmt.Errorf("Could not fit request limiting start to >= %d: %v", minStart, p)
				}
				freespace = fslist[1]
				start = freespace.Start
			}
		}

		// now align on 1MiB.  alignment actually could cost us fitting into the space.
		// For now, we just ignore that.  If input was already in units of 'align'
		// then that wont be a problem.
		if start%align != 0 {
			start = start + (align - (start % align))
		}

		if start+p.Size > freespace.Last+1 {
			return disko.Disk{}, fmt.Errorf("Alignment to %dMiB caused part to not fit start=%d: %s", align/mib, start, msg)
		}

		disk.Partitions[num] = disko.Partition{
			Start:  start,
			Last:   start + p.Size - 1,
			Number: num,
			ID:     p.ID,
			Type:   p.Type,
			Name:   p.Name,
		}
	}
	return disk, nil
}

// return a path for partition ptnum on disk path.
func pathForPartition(diskPath string, ptnum uint) string {
	base := filepath.Base(diskPath)
	sep := ""
	for _, pre := range []string{"loop", "nvme", "nbd"} {
		if strings.HasPrefix(base, pre) {
			sep = "p"
			break
		}
	}

	return diskPath + sep + fmt.Sprintf("%d", ptnum)
}

func getPartitions(disk disko.Disk) (disko.PartitionSet, error) {
	myPts := disko.PartitionSet{}

	newPts := []newPart{
		newPart{Name: espPart, Type: partid.EFI, ID: disko.GenGUID(), Size: 1 * gib},
		newPart{Name: configPart, Type: PartitionTypeIDMachineConfig, ID: disko.GenGUID(), Size: 1 * gib},
		newPart{Name: storePart, Type: PartitionTypeIDMachineStore, ID: disko.GenGUID(), Size: 60 * gib},
		newPart{Name: scratchPart, Type: PartitionTypeIDMachineScratch, ID: disko.GenGUID(), Size: 45 * gib},
	}

	// since partitionSet is a map, it gets passed by reference, and
	// changes to the disk would then propogate.  But we want the original unchanged.
	diskCopy := disk
	diskCopy.Partitions = disko.PartitionSet{}
	for k, v := range disk.Partitions {
		diskCopy.Partitions[k] = v
	}

	newDisk, err := placePartitions(diskCopy, newPts)
	if err != nil {
		return myPts, err
	}

	// placePartitions does not re-number partitions, so it is safe
	// to just check existance.
	// We want to return *only* the new partitions because disko.CreatePartition[s]
	// wipes the beginning and end of a to-be-created partition.
	for num, part := range newDisk.Partitions {
		if _, existed := disk.Partitions[num]; !existed {
			myPts[num] = part
		}
	}

	return myPts, nil
}

func makeFileSystemEFI(path string, ssize int) error {
	// -s1 copied from curtin and d-i. https://bugs.launchpad.net/bugs/1569576
	switch ssize {
	case 512, 4096:
	case 0:
		ssize = 512
	default:
		return fmt.Errorf("Sector size must be 512 or 4096. not %d", ssize)
	}

	cmd := []string{"mkfs.vfat", "-v", "-F32", "-s1", "-n", "esp"}
	if ssize != 512 {
		cmd = append(cmd, fmt.Sprintf("-S%d", ssize))
	}
	cmd = append(cmd, path)
	return mosconfig.RunCommand(cmd...)
}

func makeFileSystemEXT4(path, label string) error {
	opts := MkExt4Opts{
		Label:   label,
		ExtOpts: []string{"lazy_itable_init=0", "lazy_journal_init=0"}}
	return MkExt4FS(path, opts)
}

const mke2fsConf string = `
[defaults]
base_features = sparse_super,filetype,resize_inode,dir_index,ext_attr
default_mntopts = acl,user_xattr
enable_periodic_fsck = 0
blocksize = 4096
inode_size = 256
inode_ratio = 16384

[fs_types]
ext4 = {
	features = has_journal,extent,huge_file,flex_bg,uninit_bg,dir_nlink,extra_isize,64bit
	inode_size = 256
}
small = {
	blocksize = 1024
	inode_size = 128
	inode_ratio = 4096
}
big = {
	inode_ratio = 32768
}
huge = {
	inode_ratio = 65536
}
`

type MkExt4Opts struct {
	Label    string
	Features []string
	ExtOpts  []string
}

func MkExt4FS(path string, opts MkExt4Opts) error {
	conf, err := mosconfig.WriteTempFile("/tmp", "mkfsconf-", mke2fsConf)
	if err != nil {
		return err
	}
	defer os.Remove(conf)

	cmd := []string{"mkfs.ext4", "-F"}

	if opts.ExtOpts != nil {
		cmd = append(cmd, "-E"+strings.Join(opts.ExtOpts, ","))
	}

	if opts.Features != nil {
		cmd = append(cmd, "-O"+strings.Join(opts.Features, ","))
	}

	if opts.Label != "" {
		cmd = append(cmd, "-L"+opts.Label)
	}

	cmd = append(cmd, path)
	return mosconfig.RunCommandEnv(append(os.Environ(), "MKE2FS_CONFIG="+conf), cmd...)
}

func sortedPartNums(pSet *disko.PartitionSet) []uint {
	pNums := make([]uint, 0, len(*pSet))
	for n := range *pSet {
		pNums = append(pNums, n)
	}
	sort.Slice(pNums, func(i, j int) bool { return pNums[i] < pNums[j] })
	return pNums
}

// wipeDiskParts - Remove and wipe any partitions that are not skipped per 'skipPart'
func wipeDiskParts(disk disko.Disk, skipPart func(disko.Disk, uint) bool) error {
	fp, err := os.OpenFile(disk.Path, os.O_RDWR, 0)
	if err != nil {
		return err
	}

	delParts := []uint{}

	for _, pNum := range sortedPartNums(&disk.Partitions) {
		p := disk.Partitions[pNum]
		// The point of this operation is to wipe.  Avoid out of range errors
		// that could happen as part of a bad partition table.
		if skipPart(disk, p.Number) {
			log.Infof("Skipping partition [disk:%s partition:%d]", disk.Name, p.Number)
			continue
		}
		log.Warnf("Wiping partition [disk:%s partition:%d]", disk.Name, p.Number)
		delParts = append(delParts, p.Number)
		end := p.Last
		if end > disk.Size {
			end = disk.Size
		}

		if err := zeroStartEnd(fp, int64(p.Start), int64(end)); err != nil {
			fp.Close()
			return err
		}
	}

	if err := fp.Close(); err != nil {
		return errors.Wrap(err, "Failed to close a filehandle?")
	}

	if err := mosconfig.RunCommand("udevadm", "settle"); err != nil {
		log.Warnf("Failed udevadm settle after wipingParts")
	}

	mysys := linux.System()
	for _, n := range delParts {
		if err := mysys.DeletePartition(disk, n); err != nil {
			return err
		}
	}

	return nil
}

// zeroStartEnd - zero the start and end provided with 1MiB bytes of zeros.
func zeroStartEnd(fp io.WriteSeeker, start int64, last int64) error {
	if last <= start {
		return fmt.Errorf("last %d < start %d", last, start)
	}

	wlen := int64(disko.Mebibyte)
	bufZero := make([]byte, wlen)

	// 3 cases.
	// a.) start + wlen < last - wlen (two full writes)
	// b.) start + wlen >= last (one possibly short write)
	// c.) start + wlen >= last - wlen (overlapping zero ranges)
	type ws struct{ start, size int64 }
	var writes = []ws{{start, wlen}, {last - wlen, wlen}}
	var wnum int
	var err error

	if start+wlen >= last {
		writes = []ws{{start, last - start}}
	} else if start+wlen >= last-wlen {
		writes = []ws{{start, wlen}, {start + wlen, last - (start + wlen)}}
	}

	for _, w := range writes {
		if _, err = fp.Seek(w.start, io.SeekStart); err != nil {
			return fmt.Errorf("failed to seek to %d to write %v", w.start, w)
		}

		wnum, err = fp.Write(bufZero[:w.size])
		if err != nil {
			return fmt.Errorf("failed to write %v", w)
		}

		if int64(wnum) != w.size {
			return fmt.Errorf("wrote only %d bytes of %v", wnum, w)
		}
	}

	return nil
}

// DiskPart - a disk and partition number pair.
type DiskPart struct {
	Disk disko.Disk
	PNum uint
}

type factoryParts struct {
	Disk      disko.Disk
	PlainNum  uint
	SecureNum uint
}

// findFactoryParts - search provided diskSet and return a factoryParts
func findFactoryParts(disks disko.DiskSet) (factoryParts, error) {
	foundCrypt := []DiskPart{}
	foundPlain := []DiskPart{}
	retParts := factoryParts{}

	for _, d := range disks {
		if d.Table != disko.GPT {
			continue
		}
		for _, p := range d.Partitions {
			newDisk := DiskPart{
				Disk: d,
				PNum: p.Number,
			}
			if p.Type == trust.PBFPartitionTypeID {
				foundPlain = append(foundPlain, newDisk)
			} else if p.Type == trust.SBFPartitionTypeID {
				foundCrypt = append(foundCrypt, newDisk)
			}
		}
	}

	if len(foundPlain) == 0 && len(foundCrypt) == 0 {
		return retParts, errNoFactoryFound
	} else if len(foundPlain) == 1 && len(foundCrypt) == 1 {
		c := foundCrypt[0]
		p := foundPlain[0]
		if p.Disk.Name != c.Disk.Name {
			return retParts, fmt.Errorf(
				"Plain (%s) and Secure (%s) factory partitions found on different disks",
				pathForPartition(p.Disk.Name, p.PNum),
				pathForPartition(p.Disk.Name, c.PNum),
			)
		}
		retParts.Disk = c.Disk
		retParts.PlainNum = p.PNum
		retParts.SecureNum = c.PNum
		return retParts, nil
	}

	fdesc := func(parts []DiskPart, label string) string {
		s := fmt.Sprintf("%d %s", len(parts), label)
		if len(parts) == 0 {
			return s
		}
		paths := []string{}
		for _, p := range parts {
			paths = append(paths, pathForPartition(p.Disk.Name, p.PNum))
		}
		return fmt.Sprintf("%s (%s)", s, strings.Join(paths, ", "))
	}

	return retParts, fmt.Errorf("Expected 1 PBF and 1 SBF. Found %s %s",
		fdesc(foundPlain, "PBF"), fdesc(foundCrypt, "SBF"))
}

// findInstallDisk - find a disk to install onto.
func findInstallDisk(disks disko.DiskSet) (disko.Disk, error) {
	candList := []string{}
	for name := range disks {
		candList = append(candList, name)
	}
	sort.Strings(candList)

	for _, name := range candList {
		d := disks[name]
		if d.Size < minDiskSpace {
			log.Infof("Skipping disk %s as it is too small", d.Name)
			continue
		}
		return d, nil
	}

	return disko.Disk{}, fmt.Errorf("Did not find a valid disk for install.")
}

// doPartition sets up partitions for
// 1. EFI (1G)
// 2. Machine config - configuration backing store (/config, 1G)
// 3. Machine store - OCI backing store (/atomfs-store, 60G).
// 4. Space for overlay scratch-writes (/scratch-writes, 45G),
// It avoids touching the disk with PBF and SBF, or < 110G.  Any other disk it
// will wipe.
//
// The PreInstall() function will, during initrd, have set up a "machine:luks"
// key in the keyring.  We will use that to encrypt all three partitions.
// (XXX ^ TODO)
//
// Obviously this should become a lot more flexible.  What's here
// suffices for enabling/testing the core functionality.
func doPartition(opts mosconfig.InstallOpts) error {
	luksKey, err := mosconfig.ReadKeyFromUserKeyring("machine:luks")
	if err != nil {
		return err
	}
	log.Warnf("XXX luks key is %x, DEBUG DROP ME", luksKey)
	log.Warnf("XXX Note that we are not setting up luks yet")

	mysys := linux.System()
	disks, err := mysys.ScanAllDisks(
		func(d disko.Disk) bool {
			return true
		})
	if err != nil {
		return err
	}
	if len(disks) == 0 {
		err = fmt.Errorf("scan returned empty disk set")
	}

	factory, err := findFactoryParts(disks)
	if err != nil {
		return fmt.Errorf("Did not find factory data: %v", err)
	}

	installDisk := factory.Disk
	// if factory disk matches size need, use it for install.
	if factory.Disk.Size < minDiskSpace {
		installDisk, err = findInstallDisk(disks)
		if err != nil {
			return err
		}
	}

	if installDisk.Name == factory.Disk.Name {
		err := wipeDiskParts(factory.Disk,
			// skip the factory partitions
			func(d disko.Disk, p uint) bool {
				return p == factory.PlainNum || p == factory.SecureNum
			})
		if err != nil {
			return fmt.Errorf("Failed to remove partitions from %s: %v", factory.Disk.Name, err)
		}
	} else {
		if err := mysys.Wipe(installDisk); err != nil {
			return fmt.Errorf("Failed to wipe disk %s: %v", installDisk.Name, err)
		}
	}

	disk, err := mysys.ScanDisk(installDisk.Path)
	if err != nil {
		return errors.Wrapf(err, "Failed reading new partition table on %s", installDisk.Path)
	}

	// partition the first
	parts, err := getPartitions(disk)
	if err != nil {
		return errors.Wrapf(err, "Failed calculating partition table")
	}

	if err := mysys.CreatePartitions(disk, parts); err != nil {
		return errors.Wrapf(err, "Failed creating partitions %#v on %#v", parts, disk)
	}

	if disk, err = mysys.ScanDisk(disk.Path); err != nil {
		return errors.Wrapf(err, "Failed reading new partition table")
	}

	byName := map[string]disko.Partition{}
	for _, p := range parts {
		byName[p.Name] = p
	}

	// create and mount EFI
	efiPath := pathForPartition(disk.Path, byName[espPart].Number)
	if err := makeFileSystemEFI(efiPath, int(disk.SectorSize)); err != nil {
		return errors.Wrapf(err, "Failed creating EFI partition")
	}
	dest := filepath.Join(opts.RFS, "boot/efi")
	if err := mosconfig.EnsureDir(dest); err != nil {
		return errors.Wrapf(err, "Failed creating boot/EFI")
	}
	if err := syscall.Mount(efiPath, dest, "vfat", 0, ""); err != nil {
		return errors.Wrapf(err, "Failed mounting newly created EFI")
	}

	// create and mount /config
	configPath := pathForPartition(disk.Path, byName[configPart].Number)
	if err := makeFileSystemEXT4(configPath, configPart); err != nil {
		return errors.Wrapf(err, "Failed creating ext4 on %s", configPath)
	}
	if err := mosconfig.EnsureDir(opts.ConfigDir); err != nil {
		return errors.Wrapf(err, "Failed creating mount path %s", opts.ConfigDir)
	}
	if err := syscall.Mount(configPath, opts.ConfigDir, "ext4", 0, ""); err != nil {
		return errors.Wrapf(err, "Failed mounting newly created config")
	}

	// create and mount /atomfs-store
	// XXX  - should we rename atomfs-store to just /store?
	storePath := pathForPartition(disk.Path, byName[storePart].Number)
	if err := makeFileSystemEXT4(storePath, storePart); err != nil {
		return errors.Wrapf(err, "Failed creating ext4 on %s", storePath)
	}
	if err := mosconfig.EnsureDir(opts.StoreDir); err != nil {
		return errors.Wrapf(err, "Failed creating mount path %s", opts.StoreDir)
	}
	if err := syscall.Mount(storePath, opts.StoreDir, "ext4", 0, ""); err != nil {
		return errors.Wrapf(err, "Failed mounting newly created store")
	}

	// create and mount /scratch-writes
	scratchPath := pathForPartition(disk.Path, byName[scratchPart].Number)
	if err := makeFileSystemEXT4(scratchPath, scratchPart); err != nil {
		return errors.Wrapf(err, "Failed creating ext4 on %s", scratchPath)
	}
	dest = filepath.Join(opts.RFS, "scratch-writes")
	if err := mosconfig.EnsureDir(dest); err != nil {
		return errors.Wrapf(err, "Failed creating mount path %s", dest)
	}
	if err := syscall.Mount(scratchPath, dest, "ext4", 0, ""); err != nil {
		return errors.Wrapf(err, "Failed mounting newly created scratch-writes")
	}

	return nil
}

func doInstall(ctx *cli.Context) error {
	opts := mosconfig.InstallOpts{
		RFS:       ctx.String("rfs"),
		StoreDir:  "/atomfs-store",
		ConfigDir: "/config",
		CaPath:    "/factory/secure/manifestCA.pem",
	}

	if ctx.IsSet("rfs") {
		opts.CaPath = filepath.Join(opts.RFS, opts.CaPath)
		opts.ConfigDir = filepath.Join(opts.RFS, opts.ConfigDir)
		opts.StoreDir = filepath.Join(opts.RFS, opts.StoreDir)
	}

	if ctx.Bool("partition") {
		if err := doPartition(opts); err != nil {
			return errors.Wrapf(err, "Failed partitioning")
		}
	} else {
		if !mosconfig.PathExists(opts.CaPath) {
			return errors.Errorf("Install manifest CA (%s) missing", opts.CaPath)
		}
		if !mosconfig.PathExists(opts.ConfigDir) {
			return errors.Errorf("Configuration directory (%s) missing", opts.ConfigDir)
		}
		if !mosconfig.PathExists(opts.StoreDir) {
			return errors.Errorf("Storage cache dir (%s) missing", opts.StoreDir)
		}
	}
	if err := mosconfig.InitializeMos(ctx, opts); err != nil {
		return err
	}

	return nil
}

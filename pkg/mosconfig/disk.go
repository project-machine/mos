package mosconfig

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/anuvu/disko/partid"
	"github.com/apex/log"
	"github.com/pkg/errors"
	"github.com/rekby/gpt"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

func getBlockDevSize(dev string) (uint64, error) {
	path := path.Join("/sys/block", path.Base(dev), "queue/logical_block_size")
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return uint64(0),
			errors.Wrapf(err, "Failed to read size for '%s'", dev)
	}
	d := strings.TrimSpace(string(content))
	v, err := strconv.Atoi(d)
	if err != nil {
		return uint64(0),
			errors.Wrapf(err,
				"getBlockDevSize(%s): failed to convert '%s' to int", dev, d)
	}
	return uint64(v), nil
}

const PartLinux = 131 // 0x83, linux partition type

func isInList(list []string, e string) bool {
	for _, p := range list {
		if e == p {
			return true
		}
	}
	return false
}

func prefixInList(list []string, e string) bool {
	for _, p := range list {
		if strings.HasPrefix(e, p) {
			return true
		}
	}
	return false
}

func fetchDiskCandidates() ([]string, error) {
	excludes := []string{"dm-", "loop", "nbd", "ram", "sr"}
	candidates := []string{"/dev/sda", "/dev/sdb", "/dev/vda", "/dev/vdb"}

	more, err := ioutil.ReadDir("/sys/block")
	if err != nil {
		return candidates, errors.Wrap(err, "Failed reading /sys/block")
	}

	for _, dev := range more {
		name := dev.Name()
		devname := filepath.Join("/dev", name)
		if isInList(candidates, devname) {
			continue
		}
		if prefixInList(excludes, name) {
			continue
		}
		candidates = append(candidates, devname)
	}

	existing := []string{}

	for _, p := range candidates {
		if PathExists(p) {
			existing = append(existing, p)
		}
	}

	return existing, nil
}

func bootDiskInfo(candidates ...string) (BootDisk, error) {

	log.Infof("bootDisk: searching for boot disk in candidates: %s", strings.Join(candidates, ", "))

	isGptEsp := func(p gpt.Partition) bool {
		return p.Name() == "esp" || p.Type == gpt.PartType(partid.EFI)
	}

	found := func(disk string, espNum int) (BootDisk, error) {
		log.Infof("bootDisk: found disk=%s espNum=%d", disk, espNum)

		return BootDisk{
			Disk: disk,
			Esp:  espNum,
		}, nil
	}

	for _, disk := range candidates {
		fh, err := os.Open(disk)
		if err != nil {
			log.Debugf("bootDisk: failed to open %s, skipping.", disk)
			continue
		}
		defer fh.Close()

		sz, err := getBlockDevSize(disk)
		if err != nil {
			log.Warnf("getBlockDevSize(%s) failed: %s", disk, err)
			continue
		}

		// https://github.com/rekby/gpt/issues/2
		if _, err := fh.Seek(int64(sz), io.SeekStart); err != nil {
			log.Infof("bootDisk: failed to seek into blockdev %s: %v", disk, err)
			continue
		}

		gptTable, err := gpt.ReadTable(fh, sz)
		if err != nil {
			log.Infof("bootDisk: failed to read gptTable on disk %s: %v", disk, err)
			continue
		}

		espNum := 0
		for n, p := range gptTable.Partitions {
			if p.IsEmpty() {
				continue
			}
			if espNum == 0 && isGptEsp(p) {
				espNum = n + 1
			}
		}

		if espNum != 0 {
			return found(disk, espNum)
		} else {
			log.Debugf("%s: no boot information found", disk)
			continue
		}

		// Here we had a bbp or a esp but no /boot.
		log.Errorf("Disk %s had GPT with ESP part %d\n",
			disk, espNum)
	}

	return BootDisk{}, fmt.Errorf("Did not find boot disks searched %s", strings.Join(candidates, ", "))
}

type BootDisk struct {
	Disk string // The disk, e.g. /dev/sda
	Esp  int    // The partition number of ESP
}

// BootDiskInfo returns full dev paths, eg /dev/sda and /dev/sda1
//
// * bootdisk (where grub needs installing to)
//   - must be GPT, the value should be the disk with a BBP partition.
//
// * efiPart - well defined efi partition (type is well known id).
//   - finding this is straight forward. by ID.
func BootDiskInfo() (BootDisk, error) {
	candidates, err := fetchDiskCandidates()
	if err != nil {
		return BootDisk{}, err
	}
	return bootDiskInfo(candidates...)
}

func EspBootPartition() (string, int, error) {
	bi, err := BootDiskInfo()
	if err != nil {
		return "", -1, err
	}
	return bi.Disk, bi.Esp, err
}

func efiClearBootEntries() error {
	args := []string{"efibootmgr", "-v"}
	stdout, stderr, rc := RunCommandWithOutputErrorRc(args...)
	if rc != 0 {
		return errors.Errorf("Error querying existing boot entries:\nstdout: %s\nstderr: %s", stdout, stderr)
	}
	lines := strings.Split(string(stdout), "\n")
	for _, l := range lines {
		if !strings.HasPrefix(l, "Boot0") {
			continue
		}
		num, err := strconv.ParseInt(l[4:8], 16, 64)
		if err != nil {
			log.Warnf("Failed to parse boot entry: %s", l)
			continue
		}
		log.Infof("Removing boot entry %d (%s)", num, l)
		err = RunCommand("efibootmgr", "--bootnum", l[4:8], "--delete-bootnum")
		if err != nil {
			log.Warnf("(Probably ok) Error removing boot entry: %s", l)
		}
	}

	const enoent = "No such file or directory"
	err := RunCommand("efibootmgr", "--delete-bootnext")
	if err != nil && err.Error() != enoent {
		log.Warnf("(Probably ok) Error running efibootmgr delete-bootnext: '%s'", err.Error())
	}
	err = RunCommand("efibootmgr", "--delete-bootorder")
	if err != nil && err.Error() != enoent {
		log.Warnf("(Probably ok) Error running efibootmgr delete-bootorder: '%s'", err.Error())
	}
	return nil
}

func toUCS2(input string) ([]byte, error) {
	t := unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM).NewEncoder()
	outstr, _, err := transform.String(t, input)
	if err != nil {
		return []byte{}, errors.Wrapf(err, "Failed converting EFI boot arguments")
	}
	outbytes := []byte(outstr)
	outbytes = append(outbytes, byte(0))
	outbytes = append(outbytes, byte(0))
	final := []byte{}
	for i := 0; i+1 < len(outbytes); i += 2 {
		final = append(final, outbytes[i+1], outbytes[i])
	}
	return final, nil
}

func WriteBootEntry() error {

	bootDisk, efiPartNum, err := EspBootPartition()
	if err != nil {
		return err
	}

	shimPath := "\\efi\\boot\\shim.efi"
	argsPath := "\\efi\\boot\\kernel.efi root=soci:name=mosboot,repo=local"

	kname, err := toUCS2(argsPath)
	if err != nil {
		return errors.Wrapf(err, "Failure getting UCS2 converted shim arguments")
	}

	efiPart := fmt.Sprintf("%d", efiPartNum)
	cmd := []string{"efibootmgr", "-c", "-d", bootDisk, "-p", efiPart,
		"-L", "mosboot", "-l", shimPath, "-u", "--append-binary-args", "-"}
	return RunWithStdin(string(kname), cmd...)
}

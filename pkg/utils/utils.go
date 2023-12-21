package utils

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/jsipprell/keyctl"
	"github.com/pkg/errors"
	"github.com/plus3it/gorecurcopy"
)

func EnsureDir(dir string) error {
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return fmt.Errorf("couldn't make dirs: %w", err)
	}
	return nil
}

func IsMountpoint(path string) (bool, error) {
	return IsMountpointOfDevice(path, "")
}

func IsMountpointOfDevice(path, devicepath string) (bool, error) {
	path = strings.TrimSuffix(path, "/")
	f, err := os.Open("/proc/self/mounts")
	if err != nil {
		return false, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) <= 1 {
			continue
		}
		if (fields[1] == path || path == "") && (fields[0] == devicepath || devicepath == "") {
			return true, nil
		}
	}

	return false, nil
}

func PathExists(d string) bool {
	_, err := os.Stat(d)
	if err != nil && os.IsNotExist(err) {
		return false
	}
	return true
}

// If src is a symlink, copies content, not link.
// TODO - copy the permissions.  For now it just makes all new files
// 0644 which is what we want anyway.
func CopyFileBits(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := EnsureDir(filepath.Dir(dest)); err != nil {
		return err
	}
	out, err := os.OpenFile(dest, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return out.Close()
}

func CopyFile(src, dest string) error {
	fstat, err := os.Lstat(src)
	if err != nil {
		return fmt.Errorf("Error opening %s to copy: %w", src, err)
	}

	if (fstat.Mode() & os.ModeSymlink) == os.ModeSymlink {
		// TODO - should we?
		return fmt.Errorf("Refusing to copy symlink")
	}

	if err := EnsureDir(filepath.Dir(dest)); err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dest, os.O_RDWR|os.O_CREATE|os.O_TRUNC, fstat.Mode().Perm())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}

	// TODO - copy xattrs?

	return out.Close()
}

// if src == /tmp/a and dst == /tmp/b, and /tmp/a/x exists, then make
// sure we have /tmp/b/x.
// The way gorecurcopy.CopyDirectory() works, if $dest does not exists, it
// will fail, so create it first.
func CopyFiles(src, dest string) error {
	if !PathExists(src) {
		return fmt.Errorf("No such directory: %s", src)
	}
	if err := EnsureDir(dest); err != nil {
		return err
	}
	return gorecurcopy.CopyDirectory(src, dest)
}

func MountTmpfs(dest, size string) error {
	if err := EnsureDir(dest); err != nil {
		return fmt.Errorf("Failed making mount point: %w", err)
	}
	flags := uintptr(syscall.MS_NODEV | syscall.MS_NOSUID | syscall.MS_NOEXEC)
	err := syscall.Mount("tmpfs", dest, "tmpfs", flags, "size="+size)
	if err != nil {
		return fmt.Errorf("Failed mounting tmpfs onto %s: %w", dest, err)
	}
	return nil
}

// Create a tmpfile, write contents to it, close it, return
// the filename.
func WriteTempFile(dir, prefix, contents string) (string, error) {
	f, err := ioutil.TempFile(dir, prefix)
	if err != nil {
		return "", errors.Wrapf(err, "Failed opening a tempfile")
	}
	name := f.Name()
	_, err = f.Write([]byte(contents))
	defer f.Close()
	return name, errors.Wrapf(err, "Failed writing contents to tempfile")
}

func ShaSum(fpath string) (string, error) {
	f, err := os.Open(fpath)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func MachineDir(mname string) (string, error) {
	p, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(p, ".local", "state", "machine", "machines", mname, mname), nil
}

// UserDataDir returns the user's data directory
func UserDataDir() (string, error) {
	p, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(p, ".local", "share"), nil
}

// Get the location where keysets are stored
func GetMosKeyPath() (string, error) {
	dataDir, err := UserDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, "machine", "trust", "keys"), nil
}

// MosKeyPath returns the mos/trust key path under which all keysets
// are found.
func MosKeyPath() (string, error) {
	return GetMosKeyPath()
}

// ConfPath returns the user's config directory
func ConfPath(cluster string) string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(configDir, "machine", cluster, "machine.yaml")
}

func ReadKeyFromUserKeyring(keyName string) (string, error) {
	keyring, err := keyctl.UserKeyring()
	if err != nil {
		return "", errors.Wrapf(err, "Failed opening user keyring for reading")
	}
	key, err := keyring.Search(keyName)
	if err != nil {
		return "", errors.Wrapf(err, "Failed searching user keyring for key: %s", keyName)
	}
	b, err := key.Get()
	if err != nil {
		return "", errors.Wrapf(err, "Failed reading %s key from user keyring", keyName)
	}
	return string(b), nil
}

func KeyProjectDir(keyset, project string) (string, string, error) {
	d, err := GetMosKeyPath()
	if err != nil {
		return "", "", err
	}
	k := filepath.Join(d, keyset)
	p := filepath.Join(k, "manifest", project)
	return k, p, nil
}

// Just create a cpio file.  @path will be the top level directory
// or the file in the new cpio file index.
func NewCpio(cpio, path string) error {
	parent := filepath.Dir(path)
	target := filepath.Base(path)

	bashcmd := "cd " + parent + "; find " + target + "| cpio --create --owner=+0:+0 -H newc --quiet > " + cpio
	if err := RunCommand("/bin/bash", "-c", bashcmd); err != nil {
		return errors.Wrapf(err, "Failed creating cpio of %s -> %s", path, cpio)
	}

	return nil
}

func IsSymlink(path string) (bool, error) {
	statInfo, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	return (statInfo.Mode() & os.ModeSymlink) != 0, nil
}

func IsDirErr(path string) (bool, error) {
	if !PathExists(path) {
		return false, nil
	}
	isLink, err := IsSymlink(path)
	if err != nil {
		return false, err
	}
	if isLink {
		return false, nil
	}
	statInfo, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return statInfo.IsDir(), nil
}

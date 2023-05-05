package mosconfig

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/apex/log"
	"github.com/jsipprell/keyctl"
	"github.com/msoap/byline"
	"github.com/pkg/errors"
)

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

func EnsureDir(dir string) error {
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return fmt.Errorf("Failed creating directory %q: %w", dir, err)
	}
	return nil
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

func RunCommandEnv(env []string, args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Errorf("%s: %s: %s", strings.Join(args, " "), err, string(output))
	}
	return nil
}

func RunCommand(args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s: %s", strings.Join(args, " "), err, string(output))
	}
	return nil
}

func RunCommandWithRc(args ...string) ([]byte, int) {
	out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
	return out, GetCommandErrorRC(err)
}

func GetCommandErrorRCDefault(err error, rcError int) int {
	if err == nil {
		return 0
	}
	exitError, ok := err.(*exec.ExitError)
	if ok {
		if status, ok := exitError.Sys().(syscall.WaitStatus); ok {
			return status.ExitStatus()
		}
	}
	log.Debugf("Unavailable return code for %s. returning %d", err, rcError)
	return rcError
}

func GetCommandErrorRC(err error) int {
	return GetCommandErrorRCDefault(err, 127)
}

func LogCommand(args ...string) error {
	return LogCommandWithFunc(log.Infof, args...)
}

func LogCommandWithFunc(logf func(string, ...interface{}), args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		logf("%s-fail | %s", err)
		return err
	}
	cmd.Stderr = cmd.Stdout
	err = cmd.Start()
	if err != nil {
		logf("%s-fail | %s", args[0], err)
		return err
	}
	pid := cmd.Process.Pid
	logf("|%d-start| %q", pid, args)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		err := byline.NewReader(stdoutPipe).Each(
			func(line []byte) {
				logf("|%d-out  | %s", pid, line[:len(line)-1])
			}).Discard()
		if err != nil {
			log.Fatalf("Unexpected %s", err)
		}
		wg.Done()
	}()

	wg.Wait()
	err = cmd.Wait()

	logf("|%d-exit | rc=%d", pid, GetCommandErrorRC(err))
	return err
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

// Taken from stacker's squashfs package
// Takes /proc/self/uid_map contents as one string
// Returns true if this is a uidmap representing the whole host
// uid range.
func uidmapIsHost(oneline string) bool {
	oneline = strings.TrimSuffix(oneline, "\n")
	if len(oneline) == 0 {
		return false
	}
	lines := strings.Split(oneline, "\n")
	if len(lines) != 1 {
		return false
	}
	words := strings.Fields(lines[0])
	if len(words) != 3 || words[0] != "0" || words[1] != "0" || words[2] != "4294967295" {
		return false
	}

	return true
}

// chown of symlinks in overlay on top of squashfuse fails.  So, if we are using
// squashfuse, then find all symlinks, delete and re-reate them.
func fixupSymlinks(dir string) error {
	err := filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			log.Warnf("fixupSymlinks: failed accessing %q: %v (continuing)", path, err)
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			dest, err := os.Readlink(path)
			if err != nil {
				log.Warnf("fixupSymlinks: readlink failed on %q: %w (continuing)", path, err)
				return nil
			}
			err = os.Remove(path)
			if err != nil {
				return err
			}
			// func Symlink(oldname, newname string) error
			err = os.Symlink(dest, path)
			if err != nil {
				return err
			}
			stat := info.Sys().(*syscall.Stat_t)
			return os.Lchown(path, int(stat.Uid), int(stat.Gid))
		}
		return nil
	})

	return err
}

func dropHashAlg(d string) string {
	s := strings.SplitN(d, ":", 2)
	if len(s) == 2 {
		return s[1]
	}
	return d
}

// UserDataDir returns the user's data directory
func UserDataDir() (string, error) {
	p, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(p, ".local", "share"), nil
}

// ConfPath returns the user's config directory
func ConfPath(cluster string) string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(configDir, "machine", cluster, "machine.yaml")
}

// MosKeyPath returns the mos/trust key path under which all keysets
// are found.
func MosKeyPath() (string, error) {
	dataDir, err := UserDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, "machine", "trust", "keys"), nil
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

func RunCommandWithOutputErrorRc(args ...string) ([]byte, []byte, int) {
	cmd := exec.Command(args[0], args[1:]...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), GetCommandErrorRC(err)
}

func RunWithStdin(stdinString string, args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return errors.Errorf("%s: %s", strings.Join(args, " "), err)
	}
	go func() {
		defer stdin.Close()
		io.WriteString(stdin, stdinString)
	}()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Errorf("%s: %s: %s", strings.Join(args, " "), err, string(output))
	}
	return nil
}

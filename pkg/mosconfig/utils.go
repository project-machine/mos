package mosconfig

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	"github.com/msoap/byline"
	"github.com/apex/log"
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

//  If src is a symlink, copies content, not link.
//  TODO - copy the permissions.  For now it just makes all new files
//  0644 which is what we want anyway.
func CopyFileBits(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

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

func RunWithStdall(stdinString string, args ...string) (string, string, error) {
	cmd := exec.Command(args[0], args[1:]...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", "", fmt.Errorf("Failed getting stdin pipe %v: %w", args, err)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	go func() {
		defer stdin.Close()
		io.WriteString(stdin, stdinString)
	}()
	err = cmd.Run()
	return stdout.String(), stderr.String(), err
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

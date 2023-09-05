package trust

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/apex/log"
)

func run(args ...string) error {
	log.Debugf("Running: %s\n", args)
	cmd := exec.Command(args[0], args[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s: %s", strings.Join(args, " "), err, string(output))
	}
	log.Infof("(OK) %s: %s", strings.Join(args, " "), string(output))
	return nil
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

func RunCommandWithRc(args ...string) ([]byte, int) {
	out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
	return out, GetCommandErrorRC(err)
}

func runEnv(args []string, env []string) error {
	log.Debugf("Running: %s (environ %s\n", args, env)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = append(os.Environ(), env...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s: %s", strings.Join(args, " "), err, string(output))
	}
	log.Infof("(OK) %s: %s", strings.Join(args, " "), string(output))
	return nil
}

// runCapture - execute args and return stdout, stderr and return code.
func runCapture(args ...string) ([]byte, []byte, int) {
	cmd := exec.Command(args[0], args[1:]...) //nolint:gosec
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Run()

	log.Debugf("running: %s", strings.Join(args, " "))
	return stdout.Bytes(), stderr.Bytes(), cmd.ProcessState.ExitCode()
}

// runCaptureStdin - execute args write to stdin then return stdout, stderr and return code.
func runCaptureStdin(stdinString string, args ...string) ([]byte, []byte, int) {
	cmd := exec.Command(args[0], args[1:]...) //nolint:gosec
	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Errorf("Failed constructing stdin '%s' for %v", stdinString, args)
		return []byte{}, []byte{}, -1
	}

	go func() {
		defer stdin.Close()
		io.WriteString(stdin, stdinString)
	}()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Run()

	return stdout.Bytes(), stderr.Bytes(), cmd.ProcessState.ExitCode()
}

func RunCommandWithOutputErrorRc(args ...string) ([]byte, []byte, int) {
	cmd := exec.Command(args[0], args[1:]...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), GetCommandErrorRC(err)
}

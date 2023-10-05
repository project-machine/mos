package utils

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	"github.com/apex/log"
	"github.com/msoap/byline"
	"github.com/pkg/errors"
)

// Run the command @args, passing @stdinString on standard input.  Return
// the stdout, stderr, and any error returned.
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

// Run: run a command.  Return the output and an error if any.
func Run(args ...string) (string, error) {
	cmd := exec.Command(args[0], args[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), errors.Wrapf(err, "Failed running %v", args)
	}
	return string(output), nil
}

func RunCommandEnv(args []string, env []string) error {
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

func RunCommand(args ...string) error {
	_, err := Run(args...)
	if err != nil {
		return err
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

func RunCommandWithOutputErrorRc(args ...string) ([]byte, []byte, int) {
	cmd := exec.Command(args[0], args[1:]...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), GetCommandErrorRC(err)
}

// RunWithStdinRC - execute args write to stdin then return stdout, stderr and return code.
func RunWithStdinRC(stdinString string, args ...string) ([]byte, []byte, int) {
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

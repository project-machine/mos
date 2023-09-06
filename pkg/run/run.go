package run

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
)

// Result - return from command execution.
type Result struct {
	Args   []string
	Stdout []byte
	Stderr []byte
	RC     int
}

func (r Result) ErrorOnRCs(rcOK []int) error {
	for _, i := range rcOK {
		if r.RC == i {
			return nil
		}
	}

	return errors.New(r.String())
}

func (r Result) Error() error {
	return r.ErrorOnRCs([]int{0})
}

// String - return a formatted string for command.
func (r Result) String() string {
	tlen := len(r.Stderr)
	errEndl := ""
	if tlen == 0 || (r.Stderr)[tlen-1] != '\n' {
		errEndl = "\n"
	}

	tlen = len(r.Stdout)
	outEndl := ""
	if tlen == 0 || (r.Stdout)[tlen-1] != '\n' {
		outEndl = "\n"
	}

	return fmt.Sprintf(
		"cmd: %v\n rc: %d\n out: %s%s\n err: %s%s",
		r.Args, r.RC, r.Stdout, outEndl, r.Stderr, errEndl)
}

// Capture - execute args and Result
func Capture(args ...string) Result {
	var stdout, stderr bytes.Buffer
	var exitErr *exec.ExitError
	const unexpectedRC = -1
	var rc = unexpectedRC

	cmd := exec.Command(args[0], args[1:]...) //nolint:gosec
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	if err == nil || errors.As(err, &exitErr) {
		rc = cmd.ProcessState.ExitCode()
	}

	return Result{
		Args:   args,
		RC:     rc,
		Stdout: stdout.Bytes(),
		Stderr: stderr.Bytes(),
	}
}

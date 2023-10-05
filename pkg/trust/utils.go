package trust

import (
	"encoding/hex"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

func genPassphrase(nchars int) (string, error) {
	// each random byte will give us two characters.  We prefix with
	// trust-.  So if we want 39 or 40 characters, request (39-6)/2+1 = 17
	// bytes, giving us 136 bits of randomness.

	nbytes := (nchars-6)/2 + 1
	rand, err := HWRNGRead(nbytes)
	if err != nil {
		return "", err
	}
	s := "trust-" + hex.EncodeToString(rand)
	s = s[:nchars]
	return s, nil
}

func runWithStdin(stdinString string, args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("%s: %s", strings.Join(args, " "), err)
	}
	go func() {
		defer stdin.Close()
		io.WriteString(stdin, stdinString)
	}()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s: %s", strings.Join(args, " "), err, string(output))
	}
	return nil
}

func luksOpen(path, key, plaintextPath string) error {
	return runWithStdin(key,
		"cryptsetup", "open", "--type=luks", "--key-file=-", path, plaintextPath)
}

func luksFormatLuks2(path, key string) error {
	return runWithStdin(key, "cryptsetup", "luksFormat", "--type=luks2", "--key-file=-", path)
}

package trust

import (
	"fmt"
	"os/exec"
	"strings"

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

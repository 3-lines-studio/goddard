//go:build !unix

package heimdall

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

func execCommand(command []string, environment []string) error {
	child := exec.Command(command[0], command[1:]...)
	child.Env = environment
	child.Stdin = os.Stdin
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	if err := child.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return fmt.Errorf("no pude correr %s: salió con %d", command[0], exit.ExitCode())
		}
		return fmt.Errorf("no pude correr %s: %v", command[0], err)
	}
	return nil
}

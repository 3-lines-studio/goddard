//go:build unix

package heimdall

import (
	"fmt"
	"os/exec"
	"syscall"
)

func execCommand(command []string, environment []string) error {
	program := command[0]
	path, err := exec.LookPath(program)
	if err != nil {
		return fmt.Errorf("no pude correr %s: %v", program, err)
	}
	if err := syscall.Exec(path, command, environment); err != nil {
		return fmt.Errorf("no pude correr %s: %v", program, err)
	}
	return nil
}

//go:build unix

package compute

import (
	"os/exec"
	"syscall"
)

// ownGroup puts the command in a process group of its own, so that cancelling
// it takes down what it started and not only the shell that ran it.
func ownGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
}

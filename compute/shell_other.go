//go:build !unix

package compute

import "os/exec"

// ownGroup is the default of `exec` where there is no process group to own:
// the command is cancelled on its own.
func ownGroup(cmd *exec.Cmd) {}

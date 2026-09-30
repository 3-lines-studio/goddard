//go:build linux

package axe

import "syscall"

const prSetDumpable = 4

func SetNonDumpable() bool {
	_, _, errno := syscall.Syscall(syscall.SYS_PRCTL, prSetDumpable, 0, 0)
	return errno == 0
}

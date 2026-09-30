//go:build !linux

package axe

func SetNonDumpable() bool {
	return false
}

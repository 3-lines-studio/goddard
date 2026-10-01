// Package machine reads what the container is using right now: the memory of
// its cgroup, the volume it writes to and how many processes are around. The
// memory comes from the cgroup, so it is the one whoever watches the service
// from outside sees — page cache and slab included, not just the heap of the
// processes — which is why it is broken down: to tell whether what grows is us
// or the cache of a build.
package machine

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const cgroup = "/sys/fs/cgroup"

type Machine struct {
	Memory    Memory `json:"memory"`
	Disk      Disk   `json:"disk"`
	Processes int    `json:"processes"`
}

type Memory struct {
	Used   uint64 `json:"used"`
	Total  uint64 `json:"total"`
	Anon   uint64 `json:"anon"`
	Cache  uint64 `json:"cache"`
	Kernel uint64 `json:"kernel"`
}

type Disk struct {
	Used  uint64 `json:"used"`
	Total uint64 `json:"total"`
}

// Usage is the machine as it is when it is asked.
func Usage(volume string) Machine {
	return Machine{Memory: memory(), Disk: disk(volume), Processes: processes()}
}

func memory() Memory {
	stat, _ := os.ReadFile(filepath.Join(cgroup, "memory.stat"))
	current, _ := os.ReadFile(filepath.Join(cgroup, "memory.current"))
	maximum, _ := os.ReadFile(filepath.Join(cgroup, "memory.max"))
	text := string(stat)
	return Memory{
		Used:   number(string(current)),
		Total:  number(string(maximum)),
		Anon:   field(text, "anon"),
		Cache:  field(text, "file"),
		Kernel: field(text, "kernel"),
	}
}

// disk is the volume the workspace lives on, which is the one that fills up.
// It is the blocks minus the free ones, the same count `df` gives.
func disk(volume string) Disk {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(volume, &stat); err != nil {
		return Disk{}
	}
	block := uint64(stat.Frsize)
	return Disk{Used: (stat.Blocks - stat.Bfree) * block, Total: stat.Blocks * block}
}

func processes() int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		name := entry.Name()
		if name == "" || name[0] < '0' || name[0] > '9' {
			continue
		}
		count++
	}
	return count
}

// number is the number written there, or zero: a cap with no limit is called
// `max`.
func number(text string) uint64 {
	value := uint64(0)
	for _, character := range strings.TrimSpace(text) {
		if character < '0' || character > '9' {
			return 0
		}
		value = value*10 + uint64(character-'0')
	}
	return value
}

// field is the line of `memory.stat` that starts with that name.
func field(stat, name string) uint64 {
	for _, line := range strings.Split(stat, "\n") {
		key, value, found := strings.Cut(line, " ")
		if found && key == name {
			return number(value)
		}
	}
	return 0
}

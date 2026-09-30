package axe

import (
	"os"
	"sort"
	"strings"
)

type MachineEntry struct {
	Name  string
	IsDir bool
	Size  uint64
}

type Machine interface {
	Read(path string) ([]byte, error)
	Write(path string, bytes []byte) error
	List(path string) ([]MachineEntry, error)
	Remove(path string) error
	Run(command string, timeout uint64, progress Progress) string
}

func Resolve(dir, path string) string {
	if strings.HasPrefix(path, "/") || dir == "" {
		return path
	}
	return strings.TrimRight(dir, "/") + "/" + path
}

type Local struct {
	dir string
}

func NewLocal(dir string) *Local {
	return &Local{dir: dir}
}

func (l *Local) Read(path string) ([]byte, error) {
	return os.ReadFile(Resolve(l.dir, path))
}

func (l *Local) Write(path string, bytes []byte) error {
	return AtomicWrite(Resolve(l.dir, path), bytes)
}

func (l *Local) List(path string) ([]MachineEntry, error) {
	target := Resolve(l.dir, path)
	if target == "" {
		target = "."
	}
	items, err := os.ReadDir(target)
	if err != nil {
		return nil, err
	}
	entries := make([]MachineEntry, 0, len(items))
	for _, item := range items {
		info, err := item.Info()
		if err != nil {
			return nil, err
		}
		entries = append(entries, MachineEntry{
			Name:  item.Name(),
			IsDir: info.IsDir(),
			Size:  uint64(info.Size()),
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

func (l *Local) Remove(path string) error {
	target := Resolve(l.dir, path)
	info, err := os.Lstat(target)
	if err == nil && info.IsDir() {
		return os.RemoveAll(target)
	}
	return os.Remove(target)
}

func (l *Local) Run(command string, timeout uint64, progress Progress) string {
	return RunShell(l.dir, command, timeout, progress)
}

package compute

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/3-lines-studio/goddard/axe"
)

const (
	maxOutput          = 16 * 1024
	defaultBashTimeout = 120
	execTimeout        = 60 * time.Second
)

type Machine struct {
	channel Channel
	dir     string
}

func NewMachine(channel Channel, dir string) *Machine {
	return &Machine{channel: channel, dir: dir}
}

func (m *Machine) Read(path string) ([]byte, error) {
	result, err := m.exec(context.Background(), "cat -- "+quote(axe.Resolve(m.dir, path)), nil)
	if err != nil {
		return nil, err
	}
	if result.Exit != 0 {
		return nil, failure(result)
	}
	return result.Stdout, nil
}

func (m *Machine) Write(path string, bytes []byte) error {
	target := axe.Resolve(m.dir, path)
	command := "mkdir -p -- " + quote(filepath.Dir(target)) + " && cat > " + quote(target)
	result, err := m.exec(context.Background(), command, bytes)
	if err != nil {
		return err
	}
	if result.Exit != 0 {
		return failure(result)
	}
	return nil
}

func (m *Machine) Stat(path string) (axe.MachineEntry, error) {
	result, err := m.exec(context.Background(), "stat -c '%s %F' -- "+quote(axe.Resolve(m.dir, path)), nil)
	if err != nil {
		return axe.MachineEntry{}, err
	}
	if result.Exit != 0 {
		return axe.MachineEntry{}, failure(result)
	}
	size, kind, found := strings.Cut(strings.TrimSpace(string(result.Stdout)), " ")
	if !found {
		return axe.MachineEntry{}, fmt.Errorf("%s: no pude leer su estado", path)
	}
	bytes, err := strconv.ParseUint(size, 10, 64)
	if err != nil {
		return axe.MachineEntry{}, fmt.Errorf("%s: %v", path, err)
	}
	return axe.MachineEntry{Name: filepath.Base(path), IsDir: kind == "directory", Size: bytes}, nil
}

func (m *Machine) List(path string) ([]axe.MachineEntry, error) {
	target := axe.Resolve(m.dir, path)
	if target == "" {
		target = "."
	}
	command := "find -- " + quote(target) + ` -mindepth 1 -maxdepth 1 -printf '%y\t%s\t%f\n'`
	result, err := m.exec(context.Background(), command, nil)
	if err != nil {
		return nil, err
	}
	if result.Exit != 0 {
		return nil, failure(result)
	}
	entries := []axe.MachineEntry{}
	for _, line := range strings.Split(strings.TrimSuffix(string(result.Stdout), "\n"), "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) != 3 {
			continue
		}
		size, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			size = 0
		}
		entries = append(entries, axe.MachineEntry{Name: fields[2], IsDir: fields[0] == "d", Size: size})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

func (m *Machine) Remove(path string) error {
	result, err := m.exec(context.Background(), "rm -rf -- "+quote(axe.Resolve(m.dir, path)), nil)
	if err != nil {
		return err
	}
	if result.Exit != 0 {
		return failure(result)
	}
	return nil
}

func (m *Machine) Run(command string, timeout uint64, _ axe.Progress) string {
	seconds := timeout
	if seconds == 0 {
		seconds = defaultBashTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(seconds)*time.Second)
	defer cancel()
	script := ""
	if m.dir != "" {
		script += "mkdir -p -- " + quote(m.dir) + " && cd -- " + quote(m.dir) + " || exit 1\n"
	}
	script += "{ " + command + "\n} 2>&1"
	result, err := m.channel.Exec(ctx, "bash -s", []byte(script))
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Sprintf("error: command timed out after %d seconds", seconds)
		}
		return "error: " + err.Error()
	}
	display := axe.Sanitize(string(result.Stdout) + string(result.Stderr))
	if len(display) > maxOutput {
		display = display[len(display)-maxOutput:]
		display += "\n\n[Output truncated to the last 16KB.]"
	}
	if result.Exit != 0 {
		if display != "" && !strings.HasSuffix(display, "\n") {
			display += "\n"
		}
		display += "error: " + status(result.Exit)
	}
	return display
}

func (m *Machine) exec(ctx context.Context, command string, stdin []byte) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()
	return m.channel.Exec(ctx, command, stdin)
}

func failure(result Result) error {
	text := strings.TrimSpace(string(result.Stderr))
	if text == "" {
		text = status(result.Exit)
	}
	return errors.New(text)
}

func status(exit int) string {
	if exit < 0 {
		return fmt.Sprintf("signal: %d", -exit)
	}
	return fmt.Sprintf("exit status %d", exit)
}

func quote(text string) string {
	return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
}

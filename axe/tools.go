package axe

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	maxOutput          = 16 * 1024
	defaultBashTimeout = 120
	killGrace          = 5 * time.Second
	maxChildProcesses  = 32
)

const (
	bashSchema  = `{"type":"object","properties":{"command":{"type":"string","description":"bash command to run"},"timeout":{"type":"integer","description":"Timeout in seconds (default: 120)"}},"required":["command"]}`
	readSchema  = `{"type":"object","properties":{"path":{"type":"string","description":"Path to the file to read (relative or absolute)"},"offset":{"type":"integer","description":"Line number to start reading from (1-indexed)"},"limit":{"type":"integer","description":"Maximum number of lines to read"}},"required":["path"]}`
	writeSchema = `{"type":"object","properties":{"path":{"type":"string","description":"Path to the file to write (relative or absolute)"},"content":{"type":"string","description":"Content to write to the file"}},"required":["path","content"]}`
	editSchema  = `{"type":"object","properties":{"path":{"type":"string","description":"Path to the file to edit (relative or absolute)"},"edits":{"type":"array","description":"One or more targeted replacements. Each edit is matched against the original file, not incrementally. Do not include overlapping or nested edits. If two changes touch the same block or nearby lines, merge them into one edit instead.","items":{"type":"object","properties":{"oldText":{"type":"string","description":"Exact text for one targeted replacement. It must be unique in the original file and must not overlap with any other edits[].oldText in the same call."},"newText":{"type":"string","description":"Replacement text for this targeted edit."}},"required":["oldText","newText"]}}},"required":["path","edits"]}`
)

func BuildTools(dir string) []Tool {
	return BuildToolsOn(NewLocal(dir))
}

func BuildToolsOn(machine Machine) []Tool {
	return []Tool{
		ReadTool(machine),
		WriteTool(machine),
		EditTool(machine),
		BashTool(machine),
		SearchTool(),
		FetchTool(),
	}
}

func sanitize(s string) string {
	var builder strings.Builder
	builder.Grow(len(s))
	for _, r := range s {
		code := uint32(r)
		if code == 0x09 || code == 0x0a || code == 0x0d || !(code <= 0x1f || (code >= 0xfff9 && code <= 0xfffb)) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

var (
	childrenMu sync.Mutex
	children   = map[int]bool{}
	sigintOnce sync.Once
)

func registerPgid(pgid int) bool {
	if pgid <= 0 {
		return false
	}
	childrenMu.Lock()
	defer childrenMu.Unlock()
	if len(children) >= maxChildProcesses {
		return false
	}
	children[pgid] = true
	return true
}

func unregisterPgid(pgid int) {
	childrenMu.Lock()
	defer childrenMu.Unlock()
	delete(children, pgid)
}

func KillChildren() {
	childrenMu.Lock()
	pgids := make([]int, 0, len(children))
	for pgid := range children {
		pgids = append(pgids, pgid)
	}
	children = map[int]bool{}
	childrenMu.Unlock()
	for _, pgid := range pgids {
		syscall.Kill(-pgid, syscall.SIGKILL)
	}
}

func ensureSigintHandler() {
	sigintOnce.Do(func() {
		channel := make(chan os.Signal, 1)
		signal.Notify(channel, syscall.SIGINT)
		go func() {
			<-channel
			KillChildren()
			signal.Stop(channel)
			syscall.Kill(syscall.Getpid(), syscall.SIGINT)
		}()
	})
}

var bashTag atomic.Uint64

func RunShell(dir, command string, timeout uint64, progress Progress) string {
	ensureSigintHandler()
	tag := bashTag.Add(1) - 1
	outPath := filepath.Join(os.TempDir(), fmt.Sprintf("axe-bash-%d-%d.out", os.Getpid(), tag))
	out, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
	if err != nil {
		return "error: " + err.Error()
	}
	cmd := exec.Command("bash", "-c", command)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdout = out
	cmd.Stderr = out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		out.Close()
		os.Remove(outPath)
		return "error: " + err.Error()
	}
	pgid := cmd.Process.Pid
	if !registerPgid(pgid) {
		syscall.Kill(-pgid, syscall.SIGKILL)
		cmd.Wait()
		out.Close()
		os.Remove(outPath)
		return "error: too many live bash processes"
	}
	defer unregisterPgid(pgid)

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(time.Duration(timeout) * time.Second)
	defer deadline.Stop()

	var waitErr error
	timedOut := false
	running := true
	for running {
		select {
		case waitErr = <-done:
			running = false
		case <-ticker.C:
			if tail := readFileTail(outPath); tail != "" {
				progress.Report(sanitize(tail))
			}
		case <-deadline.C:
			syscall.Kill(-pgid, syscall.SIGKILL)
			waitErr = reap(done, killGrace)
			timedOut = true
			running = false
		}
	}
	out.Close()

	truncated := fileSize(outPath) > maxOutput
	var output string
	if truncated {
		output = readFileTail(outPath)
	} else if data, err := os.ReadFile(outPath); err == nil {
		output = string(data)
	}
	display := sanitize(output)
	if truncated {
		display += fmt.Sprintf("\n\n[Output truncated to the last 16KB. Full output: %s]", outPath)
	} else {
		os.Remove(outPath)
	}
	if timedOut {
		if display != "" && !strings.HasSuffix(display, "\n") {
			display += "\n"
		}
		display += fmt.Sprintf("error: command timed out after %d seconds", timeout)
	} else if waitErr != nil {
		if display != "" && !strings.HasSuffix(display, "\n") {
			display += "\n"
		}
		display += "error: " + statusString(waitErr)
	}
	return display
}

func reap(done <-chan error, grace time.Duration) error {
	select {
	case err := <-done:
		return err
	case <-time.After(grace):
		return nil
	}
}

func statusString(err error) string {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return err.Error()
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok {
		return err.Error()
	}
	if status.Signaled() {
		return fmt.Sprintf("signal: %d", int(status.Signal()))
	}
	return fmt.Sprintf("exit status %d", status.ExitStatus())
}

func readFileTail(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return ""
	}
	start := info.Size() - int64(maxOutput)
	if start < 0 {
		start = 0
	}
	if _, err := file.Seek(start, io.SeekStart); err != nil {
		return ""
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return ""
	}
	return string(data)
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

type bashArgs struct {
	Command string  `json:"command"`
	Timeout *uint64 `json:"timeout"`
}

func BashTool(machine Machine) Tool {
	tool := NewToolWithProgress("bash",
		"Execute a bash command in the current working directory. Returns stdout and stderr. Output is truncated to last 16KB. The default timeout is 120 seconds.",
		bashSchema,
		func(args bashArgs, progress Progress) string {
			if args.Timeout != nil && *args.Timeout == 0 {
				return "error: invalid timeout: must be a positive number of seconds"
			}
			timeout := uint64(defaultBashTimeout)
			if args.Timeout != nil {
				timeout = *args.Timeout
			}
			return machine.Run(args.Command, timeout, progress)
		})
	tool.Snippet = "Execute bash commands (ls, grep, find, etc.)"
	return tool
}

type readArgs struct {
	Path   string  `json:"path"`
	Offset *uint64 `json:"offset"`
	Limit  *uint64 `json:"limit"`
}

func ReadTool(machine Machine) Tool {
	tool := NewTool("read",
		"Read the contents of a file. Output is truncated to 16KB. Use offset/limit for large files. When you need the full file, continue with the suggested offset. Reading a JPEG, PNG, GIF, or WebP attaches the image so you can see it.",
		readSchema,
		func(args readArgs) ToolOutput {
			data, err := machine.Read(args.Path)
			if err != nil {
				return TextOutput("error: " + err.Error())
			}
			if image, ok := AttachBytes(args.Path, data); ok {
				return ToolOutput{
					Text:   fmt.Sprintf("Attached %s for viewing.", args.Path),
					Images: []Image{image},
				}
			}
			if looksBinary(data) {
				return TextOutput(fmt.Sprintf(
					"error: %s is a binary file; read handles text files and JPEG, PNG, GIF, and WebP images",
					args.Path,
				))
			}
			return TextOutput(readText(args, data))
		})
	tool.Snippet = "Read file contents (truncated, use offset to continue); images are attached so you can see them"
	return tool
}

func looksBinary(data []byte) bool {
	end := len(data)
	if end > 8192 {
		end = 8192
	}
	for _, b := range data[:end] {
		if b == 0 {
			return true
		}
	}
	return false
}

func readText(args readArgs, data []byte) string {
	start := uint64(1)
	if args.Offset != nil {
		start = *args.Offset
	}
	if start > 0 {
		start--
	}
	limit := uint64(math.MaxUint64)
	if args.Limit != nil {
		limit = *args.Limit
	}

	lines := fileLines(data)
	total := uint64(len(lines))

	var output strings.Builder
	shown := uint64(0)
	overflow := false
	var oversizedLine, oversizedBytes uint64
	for index := start; index < total; index++ {
		if shown >= limit || overflow {
			break
		}
		text := sanitize(lines[index])
		if shown == 0 && len(text) > maxOutput {
			oversizedLine, oversizedBytes = index+1, uint64(len(text))
			overflow = true
			continue
		}
		separator := 0
		if output.Len() > 0 {
			separator = 1
		}
		if output.Len()+separator+len(text) > maxOutput {
			overflow = true
			continue
		}
		if separator == 1 {
			output.WriteByte('\n')
		}
		output.WriteString(text)
		shown++
	}

	if args.Offset != nil && start >= total {
		return fmt.Sprintf("error: offset %d is beyond end of file (%d lines total)", *args.Offset, total)
	}
	if oversizedLine > 0 {
		return fmt.Sprintf("[Line %d is %d bytes, exceeds the %d limit.]", oversizedLine, oversizedBytes, maxOutput)
	}
	end := start + shown
	if end > total {
		end = total
	}
	if overflow {
		return fmt.Sprintf("%s\n\n[Showing lines %d-%d of %d (%d limit). Use offset=%d to continue.]",
			output.String(), start+1, end, total, maxOutput, end+1)
	}
	limitEnd := total
	if limit <= total-start {
		limitEnd = start + limit
	}
	if remaining := total - limitEnd; remaining > 0 {
		output.WriteString(fmt.Sprintf("\n\n[%d more lines in file. Use offset=%d to continue.]", remaining, limitEnd+1))
	}
	return output.String()
}

func fileLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

type writeArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func WriteTool(machine Machine) Tool {
	tool := NewTool("write",
		"Write content to a file. Creates the file if it doesn't exist, overwrites if it does. Automatically creates parent directories.",
		writeSchema,
		func(args writeArgs) string {
			if err := machine.Write(args.Path, []byte(args.Content)); err != nil {
				return "error: " + err.Error()
			}
			return fmt.Sprintf("wrote %s (%d bytes)", args.Path, len(args.Content))
		})
	tool.Sequential = true
	tool.Snippet = "Write content to a file"
	return tool
}

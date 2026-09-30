package axe

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	EntryMessage    = "message"
	EntryCompaction = "compaction"
	EntryUsage      = "usage"
)

type Entry struct {
	Type          string
	Message       Message
	Summary       string
	TokensBefore  int
	Timestamp     int64
	Retained      []Message
	Input         int
	Output        int
	CachedInput   int
	ContextInput  int
	ContextOutput int
}

func MessageEntry(message Message) Entry {
	return Entry{Type: EntryMessage, Message: message}
}

func CompactionEntry(summary string, tokensBefore int, timestamp int64, retained []Message) Entry {
	return Entry{
		Type:         EntryCompaction,
		Summary:      summary,
		TokensBefore: tokensBefore,
		Timestamp:    timestamp,
		Retained:     retained,
	}
}

func UsageEntry(input, output, cachedInput, contextInput, contextOutput int) Entry {
	return Entry{
		Type:          EntryUsage,
		Input:         input,
		Output:        output,
		CachedInput:   cachedInput,
		ContextInput:  contextInput,
		ContextOutput: contextOutput,
	}
}

func (e Entry) MarshalJSON() ([]byte, error) {
	switch e.Type {
	case EntryMessage:
		return encodeJSON(struct {
			Type    string  `json:"type"`
			Message Message `json:"message"`
		}{EntryMessage, e.Message})
	case EntryCompaction:
		retained := e.Retained
		if retained == nil {
			retained = []Message{}
		}
		return encodeJSON(struct {
			Type         string    `json:"type"`
			Summary      string    `json:"summary"`
			TokensBefore int       `json:"tokens_before"`
			Timestamp    int64     `json:"timestamp"`
			Retained     []Message `json:"retained"`
		}{EntryCompaction, e.Summary, e.TokensBefore, e.Timestamp, retained})
	case EntryUsage:
		return encodeJSON(struct {
			Type          string `json:"type"`
			Input         int    `json:"input"`
			Output        int    `json:"output"`
			CachedInput   int    `json:"cached_input"`
			ContextInput  int    `json:"context_input"`
			ContextOutput int    `json:"context_output"`
		}{EntryUsage, e.Input, e.Output, e.CachedInput, e.ContextInput, e.ContextOutput})
	}
	return nil, fmt.Errorf("unknown entry type %q", e.Type)
}

func (e *Entry) UnmarshalJSON(data []byte) error {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return err
	}
	switch probe.Type {
	case EntryMessage:
		var value struct {
			Message Message `json:"message"`
		}
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		*e = MessageEntry(value.Message)
	case EntryCompaction:
		var value struct {
			Summary      string    `json:"summary"`
			TokensBefore int       `json:"tokens_before"`
			Timestamp    int64     `json:"timestamp"`
			Retained     []Message `json:"retained"`
		}
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		*e = CompactionEntry(value.Summary, value.TokensBefore, value.Timestamp, value.Retained)
	case EntryUsage:
		var value struct {
			Input         int `json:"input"`
			Output        int `json:"output"`
			CachedInput   int `json:"cached_input"`
			ContextInput  int `json:"context_input"`
			ContextOutput int `json:"context_output"`
		}
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		*e = UsageEntry(value.Input, value.Output, value.CachedInput, value.ContextInput, value.ContextOutput)
	default:
		return fmt.Errorf("unknown entry type %q", probe.Type)
	}
	return nil
}

type SessionMeta struct {
	ID      string
	Title   string
	Updated int64
	Turns   int
}

type Store interface {
	Live() []Entry
	Save(entries []Entry) error
	Append(entries []Entry) error
	Archive() (string, bool)
	ContinueArchived(id string, entries []Entry) (bool, error)
	ContinueArchivedLive(id string) (bool, error)
	Discard()
	List() []SessionMeta
	Load(id string) ([]Entry, bool)
	ResumeID() (string, bool)
	SetResumeID(id string)
	ClearResumeID()
}

func storeDir(dir string) string {
	return filepath.Join(dir, "sessions")
}

func ScopeDir(dir, cwd string) string {
	if absolute, err := filepath.Abs(cwd); err == nil {
		cwd = absolute
	}
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		cwd = resolved
	}
	hash := uint64(0xcbf29ce484222325)
	for _, b := range []byte(cwd) {
		hash ^= uint64(b)
		hash *= 0x100000001b3
	}
	return filepath.Join(dir, "projects", fmt.Sprintf("%016x", hash))
}

func livePath(dir string) string {
	return filepath.Join(dir, "session.jsonl")
}

func NowMs() int64 {
	return time.Now().UnixMilli()
}

const (
	CompactionPrefix = "The conversation history before this point was compacted into the following summary:\n\n<summary>\n"
	CompactionSuffix = "\n</summary>"
)

type ContextOptions struct {
	OriginalTask   bool
	WorkspaceState bool
}

func DefaultContextOptions() ContextOptions {
	return ContextOptions{OriginalTask: true, WorkspaceState: true}
}

func ContextMessages(entries []Entry) []Message {
	return ContextMessagesWith(entries, DefaultContextOptions())
}

func ContextMessagesWith(entries []Entry, options ContextOptions) []Message {
	hasCompaction := false
	for _, entry := range entries {
		if entry.Type == EntryCompaction {
			hasCompaction = true
		}
	}
	if !hasCompaction {
		out := []Message{}
		for _, entry := range entries {
			if entry.Type == EntryMessage {
				out = append(out, entry.Message)
			}
		}
		return out
	}
	sections := []string{}
	if options.OriginalTask {
		sections = append(sections, "## Authoritative Original Task\n"+originalTask(entries))
	}
	if options.WorkspaceState {
		sections = append(sections, "## Authoritative Workspace State\n"+workspaceState(entries))
	}
	extras := strings.Join(sections, "\n\n")
	out := []Message{}
	for _, entry := range entries {
		switch entry.Type {
		case EntryMessage:
			out = append(out, entry.Message)
		case EntryCompaction:
			content := CompactionPrefix + entry.Summary + CompactionSuffix
			if extras != "" {
				content = CompactionPrefix + entry.Summary + "\n\n" + extras + CompactionSuffix
			}
			out = []Message{{Role: "user", Content: content}}
			out = append(out, entry.Retained...)
		}
	}
	return out
}

func originalTask(entries []Entry) string {
	for _, entry := range entries {
		if entry.Type != EntryMessage {
			continue
		}
		message := entry.Message
		if message.Role == "user" && !strings.HasPrefix(message.Content, CompactionPrefix) {
			return message.Content
		}
	}
	return ""
}

type workspaceArgs struct {
	Path    string
	Command string
}

func workspaceState(entries []Entry) string {
	calls := map[string]struct {
		name    string
		path    string
		command string
	}{}
	read := []string{}
	modified := []string{}
	commands := []struct {
		command string
		content string
		failed  bool
	}{}
	for _, entry := range entries {
		if entry.Type != EntryMessage {
			continue
		}
		message := entry.Message
		if message.Role == "assistant" {
			for _, call := range message.ToolCalls {
				var args workspaceArgs
				json.Unmarshal([]byte(call.Arguments), &args)
				calls[call.ID] = struct {
					name    string
					path    string
					command string
				}{call.Name, args.Path, args.Command}
			}
			continue
		}
		if message.Role != "tool" {
			continue
		}
		failed := strings.HasPrefix(strings.TrimLeft(message.Content, " \t\n\r"), "error:")
		call, present := calls[message.ToolCallID]
		if !present {
			continue
		}
		delete(calls, message.ToolCallID)
		if call.name == "bash" {
			command := call.command
			if command == "" {
				command = "unknown command"
			}
			if len(commands) == 8 {
				commands = commands[1:]
			}
			commands = append(commands, struct {
				command string
				content string
				failed  bool
			}{command, message.Content, failed})
			continue
		}
		if failed || call.path == "" {
			continue
		}
		switch call.name {
		case "read":
			read = append(read, call.path)
		case "write", "edit":
			modified = append(modified, call.path)
		}
	}
	sort.Strings(read)
	read = dedupe(read)
	sort.Strings(modified)
	modified = dedupe(modified)
	lines := make([]string, 0, len(commands))
	for _, entry := range commands {
		result := ""
		if !entry.failed && strings.TrimSpace(entry.content) == "" {
			result = "success with no output"
		} else {
			result = compactObservation(entry.content, 300)
		}
		lines = append(lines, entry.command+" => "+result)
	}
	readText := "none"
	if len(read) > 0 {
		readText = strings.Join(read, ", ")
	}
	modifiedText := "none"
	if len(modified) > 0 {
		modifiedText = strings.Join(modified, ", ")
	}
	commandsText := "none"
	if len(lines) > 0 {
		commandsText = strings.Join(lines, "\n")
	}
	return fmt.Sprintf(
		"Files read: %s\nFiles modified: %s\nRecent commands and results:\n%s",
		readText, modifiedText, commandsText,
	)
}

func dedupe(values []string) []string {
	out := make([]string, 0, len(values))
	for index, value := range values {
		if index == 0 || values[index-1] != value {
			out = append(out, value)
		}
	}
	return out
}

func parseEntryLine(line string) (Entry, bool) {
	var entry Entry
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		return Entry{}, false
	}
	return entry, true
}

func readEntries(path string) []Entry {
	file, err := os.Open(path)
	if err != nil {
		return []Entry{}
	}
	defer file.Close()
	entries := []Entry{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 32*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		entry, ok := parseEntryLine(line)
		if !ok {
			continue
		}
		entries = append(entries, entry)
	}
	return entries
}

func validID(id string) bool {
	return id != "" && id != "." && id != ".." && !strings.Contains(id, "/") && !strings.Contains(id, `\`)
}

func writeEntries(path string, entries []Entry) error {
	return atomicWriteWith(path, func(file *os.File) error {
		for _, entry := range entries {
			data, err := encodeJSON(entry)
			if err != nil {
				return err
			}
			if _, err := file.Write(append(data, '\n')); err != nil {
				return err
			}
		}
		return nil
	})
}

func saveLive(dir string, entries []Entry) error {
	if len(entries) == 0 {
		return nil
	}
	path := livePath(dir)
	if err := writeEntries(path, entries); err != nil {
		return err
	}
	writeLiveSidecar(dir, path, entries)
	return nil
}

func appendLive(dir string, entries []Entry) error {
	if len(entries) == 0 {
		return nil
	}
	path := livePath(dir)
	if _, err := os.Stat(path); err != nil {
		if err := writeEntries(path, entries); err != nil {
			return err
		}
		writeLiveSidecar(dir, path, entries)
		return nil
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0o666)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	size := info.Size()
	if size > 0 {
		last := make([]byte, 1)
		if _, err := file.ReadAt(last, size-1); err != nil {
			return err
		}
		if last[0] != '\n' {
			complete := int64(0)
			buffer := make([]byte, 8192)
			end := size
			for {
				start := end - int64(len(buffer))
				if start < 0 {
					start = 0
				}
				count := int(end - start)
				if _, err := file.ReadAt(buffer[:count], start); err != nil {
					return err
				}
				found := int64(-1)
				for index := count - 1; index >= 0; index-- {
					if buffer[index] == '\n' {
						found = start + int64(index) + 1
						break
					}
				}
				if found >= 0 {
					complete = found
					break
				}
				if start == 0 {
					complete = 0
					break
				}
				end = start
			}
			if err := file.Truncate(complete); err != nil {
				return err
			}
		}
	}
	oldBytes := uint64(size)
	sidecar, hasSidecar := readLiveSidecar(dir, oldBytes)
	if _, err := file.Seek(0, 2); err != nil {
		return err
	}
	for _, entry := range entries {
		data, err := encodeJSON(entry)
		if err != nil {
			return err
		}
		if _, err := file.Write(append(data, '\n')); err != nil {
			return err
		}
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if hasSidecar {
		updateSidecar(&sidecar, entries)
		newInfo, err := file.Stat()
		if err == nil {
			sidecar.Bytes = uint64(newInfo.Size())
			writeLiveSidecarValue(dir, sidecar)
		}
	}
	return nil
}

func resumeIDPath(dir string) string {
	return filepath.Join(dir, "session.resume_id")
}

func setResumeID(dir, id string) {
	if !validID(id) {
		return
	}
	AtomicWrite(resumeIDPath(dir), []byte(id))
}

func clearResumeID(dir string) {
	os.Remove(resumeIDPath(dir))
}

func discardLive(dir string) {
	os.Remove(livePath(dir))
	os.Remove(liveSidecarPath(dir))
	os.Remove(filepath.Join(dir, "session.title"))
	clearResumeID(dir)
}

func loadResumeID(dir string) (string, bool) {
	data, err := os.ReadFile(resumeIDPath(dir))
	if err != nil {
		return "", false
	}
	id := strings.TrimSpace(string(data))
	if !validID(id) {
		return "", false
	}
	return id, true
}

func continueArchived(dir, id string, entries []Entry) (bool, error) {
	if !validID(id) || len(entries) == 0 {
		return false, nil
	}
	path := filepath.Join(storeDir(dir), id+".jsonl")
	if err := writeEntries(path, entries); err != nil {
		return false, err
	}
	liveTitle := filepath.Join(dir, "session.title")
	title := ""
	if data, err := os.ReadFile(liveTitle); err == nil && strings.TrimSpace(string(data)) != "" {
		title = strings.TrimSpace(string(data))
	} else if data, err := os.ReadFile(titlePath(dir, id)); err == nil && strings.TrimSpace(string(data)) != "" {
		title = strings.TrimSpace(string(data))
	} else {
		title = titleFromEntries(entries)
	}
	writeSessionSidecar(dir, id, path, title, entries)
	os.Remove(liveTitle)
	os.Remove(livePath(dir))
	os.Remove(liveSidecarPath(dir))
	clearResumeID(dir)
	return true, nil
}

func continueArchivedLive(dir, id string) (bool, error) {
	if !validID(id) {
		return false, nil
	}
	live := livePath(dir)
	info, err := os.Stat(live)
	if err != nil {
		return false, nil
	}
	sidecar, ok := readLiveSidecar(dir, uint64(info.Size()))
	if !ok {
		return false, nil
	}
	path := filepath.Join(storeDir(dir), id+".jsonl")
	if err := os.Rename(live, path); err != nil {
		return false, nil
	}
	liveTitle := filepath.Join(dir, "session.title")
	if data, err := os.ReadFile(liveTitle); err == nil && strings.TrimSpace(string(data)) != "" {
		sidecar.Title = strings.TrimSpace(string(data))
	}
	writeSessionSidecarValue(dir, id, sidecar)
	os.Remove(liveTitle)
	os.Remove(liveSidecarPath(dir))
	clearResumeID(dir)
	return true, nil
}

func loadLive(dir string) []Entry {
	return readEntries(livePath(dir))
}

func listSessions(dir string) []SessionMeta {
	items, err := os.ReadDir(storeDir(dir))
	if err != nil {
		return []SessionMeta{}
	}
	out := []SessionMeta{}
	for _, item := range items {
		if filepath.Ext(item.Name()) != ".jsonl" {
			continue
		}
		id := strings.TrimSuffix(item.Name(), ".jsonl")
		path := filepath.Join(storeDir(dir), item.Name())
		info, infoErr := item.Info()
		var updated int64
		var sidecar SessionSidecar
		hasSidecar := false
		if infoErr == nil {
			updated = info.ModTime().UnixMilli()
			sidecar, hasSidecar = readSessionSidecar(dir, id, uint64(info.Size()))
		}
		derivedTitle := ""
		derivedTurns := 0
		if hasSidecar {
			derivedTurns = sidecar.Turns
		} else {
			derivedTitle, derivedTurns = sessionSummary(path)
		}
		title := derivedTitle
		if hasSidecar {
			title = sidecar.Title
		}
		if data, err := os.ReadFile(titlePath(dir, id)); err == nil && strings.TrimSpace(string(data)) != "" {
			title = strings.TrimSpace(string(data))
		}
		out = append(out, SessionMeta{ID: id, Title: title, Updated: updated, Turns: derivedTurns})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Updated > out[j].Updated })
	return out
}

func sessionSummary(path string) (string, int) {
	file, err := os.Open(path)
	if err != nil {
		return "Untitled session", 0
	}
	defer file.Close()
	sidecar := SessionSidecar{Title: "Untitled session"}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 32*1024*1024)
	for scanner.Scan() {
		entry, ok := parseEntryLine(scanner.Text())
		if !ok {
			continue
		}
		updateSidecar(&sidecar, []Entry{entry})
	}
	return sidecar.Title, sidecar.Turns
}

func titleFromEntries(entries []Entry) string {
	for _, message := range ContextMessages(entries) {
		if message.Role == "user" && message.Content != "" {
			return firstWords(message.Content, 8)
		}
	}
	return "Untitled session"
}

func firstWords(s string, n int) string {
	words := strings.Fields(s)
	if len(words) > n {
		words = words[:n]
	}
	if len(words) == 0 {
		return "Untitled session"
	}
	return strings.Join(words, " ")
}

func archiveLive(dir string) (string, bool) {
	path := livePath(dir)
	info, err := os.Stat(path)
	if err != nil {
		return "", false
	}
	bytes := uint64(info.Size())
	liveSidecar, hasLiveSidecar := readLiveSidecar(dir, bytes)
	if id, ok := loadResumeID(dir); ok {
		if continued, err := continueArchivedLive(dir, id); err == nil && continued {
			return id, true
		} else if err != nil {
			return "", false
		}
		entries := readEntries(path)
		if continued, err := continueArchived(dir, id, entries); err == nil && continued {
			return id, true
		} else if err != nil {
			return "", false
		}
	}
	var entries []Entry
	if !hasLiveSidecar {
		entries = readEntries(path)
		if len(entries) == 0 {
			return "", false
		}
	}
	store := storeDir(dir)
	os.MkdirAll(store, 0o777)
	base := fmt.Sprintf("%d", NowMs())
	id := base
	dest := filepath.Join(store, id+".jsonl")
	for n := 1; ; n++ {
		if _, err := os.Stat(dest); err != nil {
			break
		}
		id = fmt.Sprintf("%s-%d", base, n)
		dest = filepath.Join(store, id+".jsonl")
	}
	if err := os.Rename(path, dest); err != nil {
		return "", false
	}
	liveTitlePath := filepath.Join(dir, "session.title")
	var legacyTitle string
	if data, err := os.ReadFile(liveTitlePath); err == nil && strings.TrimSpace(string(data)) != "" {
		legacyTitle = strings.TrimSpace(string(data))
	}
	sidecar := SessionSidecar{Title: "Untitled session"}
	if hasLiveSidecar {
		sidecar = liveSidecar
	} else {
		sidecar = metadataFromEntries(entries, bytes)
	}
	if legacyTitle != "" {
		sidecar.Title = legacyTitle
	}
	writeSessionSidecarValue(dir, id, sidecar)
	os.Remove(liveTitlePath)
	os.Remove(liveSidecarPath(dir))
	clearResumeID(dir)
	return id, true
}

func loadByID(dir, id string) ([]Entry, bool) {
	if !validID(id) {
		return nil, false
	}
	path := filepath.Join(storeDir(dir), id+".jsonl")
	if _, err := os.Stat(path); err != nil {
		return nil, false
	}
	return readEntries(path), true
}

type FsStore struct {
	dir string
}

func NewFsStore(dir string) *FsStore {
	return &FsStore{dir: dir}
}

func (s *FsStore) Live() []Entry                { return loadLive(s.dir) }
func (s *FsStore) Save(entries []Entry) error   { return saveLive(s.dir, entries) }
func (s *FsStore) Append(entries []Entry) error { return appendLive(s.dir, entries) }
func (s *FsStore) Archive() (string, bool)      { return archiveLive(s.dir) }
func (s *FsStore) Discard()                     { discardLive(s.dir) }
func (s *FsStore) List() []SessionMeta          { return listSessions(s.dir) }
func (s *FsStore) ResumeID() (string, bool)     { return loadResumeID(s.dir) }
func (s *FsStore) SetResumeID(id string)        { setResumeID(s.dir, id) }
func (s *FsStore) ClearResumeID()               { clearResumeID(s.dir) }

func (s *FsStore) ContinueArchived(id string, entries []Entry) (bool, error) {
	return continueArchived(s.dir, id, entries)
}

func (s *FsStore) ContinueArchivedLive(id string) (bool, error) {
	return continueArchivedLive(s.dir, id)
}

func (s *FsStore) Load(id string) ([]Entry, bool) {
	return loadByID(s.dir, id)
}

func titlePath(dir, id string) string {
	return filepath.Join(storeDir(dir), id+".title")
}

type SessionSidecar struct {
	Title string `json:"title"`
	Turns int    `json:"turns"`
	Bytes uint64 `json:"bytes"`
}

func sidecarPath(dir, id string) string {
	return filepath.Join(storeDir(dir), id+".meta")
}

func liveSidecarPath(dir string) string {
	return filepath.Join(dir, "session.meta")
}

func metadataFromEntries(entries []Entry, bytes uint64) SessionSidecar {
	sidecar := SessionSidecar{Title: "Untitled session", Bytes: bytes}
	updateSidecar(&sidecar, entries)
	return sidecar
}

func updateSidecar(sidecar *SessionSidecar, entries []Entry) {
	for _, entry := range entries {
		sidecar.Turns = nextTurns(sidecar.Turns, entry)
		switch entry.Type {
		case EntryMessage:
			if entry.Message.Role == "user" && sidecar.Title == "Untitled session" && entry.Message.Content != "" {
				sidecar.Title = firstWords(entry.Message.Content, 8)
			}
		case EntryCompaction:
			sidecar.Title = firstWords(CompactionPrefix+entry.Summary+CompactionSuffix, 8)
		}
	}
}

func writeLiveSidecar(dir, path string, entries []Entry) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	sidecar := metadataFromEntries(entries, uint64(info.Size()))
	writeLiveSidecarValue(dir, sidecar)
}

func writeLiveSidecarValue(dir string, sidecar SessionSidecar) {
	data, err := encodeJSON(sidecar)
	if err != nil {
		return
	}
	AtomicWrite(liveSidecarPath(dir), data)
}

func readLiveSidecar(dir string, bytes uint64) (SessionSidecar, bool) {
	return readSidecar(liveSidecarPath(dir), bytes)
}

func nextTurns(turns int, entry Entry) int {
	switch entry.Type {
	case EntryMessage:
		if entry.Message.Role == "user" {
			return turns + 1
		}
	case EntryCompaction:
		count := 1
		for _, message := range entry.Retained {
			if message.Role == "user" {
				count++
			}
		}
		return count
	}
	return turns
}

func entryTurns(entries []Entry) int {
	turns := 0
	for _, entry := range entries {
		turns = nextTurns(turns, entry)
	}
	return turns
}

func writeSessionSidecar(dir, id, path, title string, entries []Entry) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	sidecar := SessionSidecar{Title: title, Turns: entryTurns(entries), Bytes: uint64(info.Size())}
	writeSessionSidecarValue(dir, id, sidecar)
}

func writeSessionSidecarValue(dir, id string, sidecar SessionSidecar) {
	data, err := encodeJSON(sidecar)
	if err != nil {
		return
	}
	AtomicWrite(sidecarPath(dir, id), data)
}

func readSessionSidecar(dir, id string, bytes uint64) (SessionSidecar, bool) {
	return readSidecar(sidecarPath(dir, id), bytes)
}

func readSidecar(path string, bytes uint64) (SessionSidecar, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SessionSidecar{}, false
	}
	var sidecar SessionSidecar
	if err := json.Unmarshal(data, &sidecar); err != nil {
		return SessionSidecar{}, false
	}
	if bytes != sidecar.Bytes {
		return SessionSidecar{}, false
	}
	return sidecar, true
}

func DropIncompleteToolCalls(msgs []Message) []Message {
	results := map[string]int{}
	for index, message := range msgs {
		if message.Role == "tool" {
			if _, present := results[message.ToolCallID]; !present {
				results[message.ToolCallID] = index
			}
		}
	}
	used := make([]bool, len(msgs))
	out := make([]Message, 0, len(msgs))
	for index, message := range msgs {
		if used[index] || message.Role == "tool" {
			continue
		}
		if message.Role != "assistant" || len(message.ToolCalls) == 0 {
			out = append(out, message)
			continue
		}
		ids := make([]string, 0, len(message.ToolCalls))
		complete := true
		for _, call := range message.ToolCalls {
			target, present := results[call.ID]
			if !present || used[target] {
				complete = false
				break
			}
			ids = append(ids, call.ID)
		}
		if !complete {
			continue
		}
		out = append(out, message)
		for _, id := range ids {
			target := results[id]
			used[target] = true
			out = append(out, msgs[target])
		}
	}
	return out
}

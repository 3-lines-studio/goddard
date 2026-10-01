package axe

import (
	"context"
	"encoding/json"
	"fmt"
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

const untitled = "Untitled session"

// Store is where a conversation's history lives. It used to be a directory of
// JSONL files; a cloud axe has no volume to mount, so the only implementation
// is Postgres (PgStore) and the scope that used to be a path is a key the
// service picks.
type Store interface {
	Live(ctx context.Context) ([]Entry, error)
	Save(ctx context.Context, entries []Entry) error
	Append(ctx context.Context, entries []Entry) error
	Archive(ctx context.Context) (string, bool, error)
	ContinueArchived(ctx context.Context, id string, entries []Entry) (bool, error)
	ContinueArchivedLive(ctx context.Context, id string) (bool, error)
	Discard(ctx context.Context) error
	List(ctx context.Context) ([]SessionMeta, error)
	Load(ctx context.Context, id string) ([]Entry, bool, error)
	ResumeID(ctx context.Context) (string, bool, error)
	SetResumeID(ctx context.Context, id string) error
	ClearResumeID(ctx context.Context) error
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

func validID(id string) bool {
	return id != "" && id != "." && id != ".." && !strings.Contains(id, "/") && !strings.Contains(id, `\`)
}

func titleFromEntries(entries []Entry) string {
	for _, message := range ContextMessages(entries) {
		if message.Role == "user" && message.Content != "" {
			return firstWords(message.Content, 8)
		}
	}
	return untitled
}

func firstWords(s string, n int) string {
	words := strings.Fields(s)
	if len(words) > n {
		words = words[:n]
	}
	if len(words) == 0 {
		return untitled
	}
	return strings.Join(words, " ")
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

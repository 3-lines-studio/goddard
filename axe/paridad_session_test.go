package axe

import (
	"encoding/json"
	"strconv"
	"testing"
)

func sessionUser(content string) Message {
	return Message{Role: "user", Content: content}
}

func sessionAssistant(content string) Message {
	return Message{Role: "assistant", Content: content}
}

func sessionTool(id, content string) Message {
	return Message{Role: "tool", Content: content, ToolCallID: id}
}

func sessionCall(id, name, arguments string) Message {
	return Message{Role: "assistant", ToolCalls: []ToolCall{{ID: id, Name: name, Arguments: arguments}}}
}

func sessionEntries() []Entry {
	return []Entry{
		MessageEntry(sessionUser("tarea original")),
		MessageEntry(sessionAssistant("ok")),
		UsageEntry(10, 5, 0, 100, 20),
		CompactionEntry("resumen", 120, 7, []Message{sessionUser("reciente")}),
		UsageEntry(1, 1, 0, 200, 30),
	}
}

func sessionWork() []Entry {
	return []Entry{
		MessageEntry(sessionCall("1", "read", `{"path":"/a.txt"}`)),
		MessageEntry(sessionTool("1", "a\nb")),
		MessageEntry(sessionCall("2", "bash", `{"command":"ls -la"}`)),
		MessageEntry(sessionTool("2", "salida del comando")),
		MessageEntry(sessionCall("3", "bash", `{"command":"false"}`)),
		MessageEntry(sessionTool("3", "error: fallo")),
		MessageEntry(sessionCall("4", "write", `{"path":"/b.txt","content":"x"}`)),
		MessageEntry(sessionTool("4", "wrote /b.txt (1 bytes)")),
		MessageEntry(sessionCall("5", "edit", `{"path":"/a.txt","edits":[]}`)),
		MessageEntry(sessionTool("5", "error: boom")),
		MessageEntry(sessionCall("6", "bash", `{"command":"echo hola"}`)),
		MessageEntry(sessionTool("6", "")),
	}
}

func TestParidadSessionConRust(t *testing.T) {
	expected := readTestdata(t, "testdata/paridad-session.txt")
	entries := sessionEntries()
	work := sessionWork()

	cases := map[string]func() string{
		"json_message": func() string {
			return escapeEdit(marshal(MessageEntry(sessionUser("hola\nmundo"))))
		},
		"json_compaction": func() string {
			return escapeEdit(marshal(CompactionEntry("resumen", 10, 5, []Message{sessionUser("reciente")})))
		},
		"json_usage": func() string {
			return escapeEdit(marshal(UsageEntry(1, 2, 3, 4, 5)))
		},
		"json_usage_partial": func() string {
			var entry Entry
			if err := json.Unmarshal([]byte(`{"type":"usage","input":1,"output":2,"cached_input":3,"context_input":4}`), &entry); err != nil {
				t.Fatal(err)
			}
			return escapeEdit(marshal(entry))
		},
		"json_compaction_empty_retained": func() string {
			return escapeEdit(marshal(CompactionEntry("", 0, 0, []Message{})))
		},
		"json_bare_message_rejected": func() string {
			var entry Entry
			err := json.Unmarshal([]byte(`{"Role":"user","Content":"hi"}`), &entry)
			return boolText(err != nil)
		},
		"context_default": func() string {
			return escapeEdit(marshal(ContextMessages(entries)))
		},
		"context_minimal": func() string {
			return escapeEdit(marshal(ContextMessagesWith(entries, ContextOptions{})))
		},
		"context_no_compaction": func() string {
			return escapeEdit(marshal(ContextMessages([]Entry{MessageEntry(sessionUser("solo"))})))
		},
		"original_task": func() string { return escapeEdit(originalTask(entries)) },
		"title":         func() string { return escapeEdit(titleFromEntries(entries)) },
		"turns":         func() string { return strconv.FormatInt(int64(entryTurns(entries)), 10) },
		"latest_tokens": func() string { return optionText(LatestContextTokens(entries)) },
		"latest_tokens_after_compaction": func() string {
			return optionText(LatestContextTokens(entries[4:]))
		},
		"workspace_state":           func() string { return escapeEdit(workspaceState(work)) },
		"serialize_conversation":    func() string { return escapeEdit(serializeConversation(ContextMessages(work))) },
		"compact_observation":       func() string { return escapeEdit(compactObservation("hola mundo 🚀 adiós", 6)) },
		"compact_observation_short": func() string { return escapeEdit(compactObservation("corto", 100)) },
		"first_words":               func() string { return escapeEdit(firstWords("  uno  dos   tres cuatro", 3)) },
		"first_words_empty":         func() string { return escapeEdit(firstWords("   ", 3)) },
		"dropped_incomplete": func() string {
			dropped := []Message{
				sessionUser("hi"),
				sessionCall("1", "read", "{}"),
				sessionTool("1", "ok"),
				sessionCall("2", "read", "{}"),
				sessionAssistant("fin"),
			}
			return escapeEdit(marshal(DropIncompleteToolCalls(dropped)))
		},
		"dropped_out_of_order": func() string {
			outOfOrder := []Message{
				sessionCall("1", "a", "{}"),
				sessionCall("2", "b", "{}"),
				sessionTool("2", "B"),
				sessionTool("1", "A"),
			}
			return escapeEdit(marshal(DropIncompleteToolCalls(outOfOrder)))
		},
		"dropped_orphan": func() string {
			orphan := []Message{sessionUser("hi"), sessionTool("9", "suelto"), sessionAssistant("ok")}
			return escapeEdit(marshal(DropIncompleteToolCalls(orphan)))
		},
		"overflow_true":       func() string { return boolText(IsOverflowError("This model's maximum context length is 8192 tokens")) },
		"overflow_rate_limit": func() string { return boolText(IsOverflowError("Rate limit reached: maximum context length")) },
		"overflow_false":      func() string { return boolText(IsOverflowError("connection reset")) },
		"valid_summary_short": func() string { return boolText(validSummary("## Goal\n- x")) },
	}

	for name := range expected {
		if _, covered := cases[name]; !covered {
			t.Errorf("el dump trae el caso %q y nadie lo cubre", name)
		}
	}
	for name, want := range expected {
		check, covered := cases[name]
		if !covered {
			continue
		}
		if got := check(); got != want {
			t.Errorf("%s:\n got:  %s\n want: %s", name, got, want)
		}
	}
}

func boolText(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func optionText(value int, ok bool) string {
	if !ok {
		return "None"
	}
	return "Some(" + strconv.FormatInt(int64(value), 10) + ")"
}

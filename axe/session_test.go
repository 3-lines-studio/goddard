package axe

import (
	"strings"
	"testing"
)

type summaryProvider struct {
	content  string
	requests []string
}

func (s *summaryProvider) Complete(req *Request) (*Response, error) {
	if len(req.Messages) > 0 {
		s.requests = append(s.requests, req.Messages[0].Content)
	}
	content := s.content
	if content == "" {
		var builder strings.Builder
		for _, heading := range summaryHeadings {
			builder.WriteString(heading + "\n- fact\n")
		}
		content = builder.String()
	}
	return &Response{Message: Message{Role: "assistant", Content: content}, StopReason: "stop"}, nil
}

func (s *summaryProvider) Stream(*Request) *StreamHandle {
	panic("summary provider only completes")
}

func TestRetainedContextUsesUsageAndCompleteTurns(t *testing.T) {
	call := sessionCall("call-1", "read", `{"path":"src/main.rs"}`)
	entries := []Entry{
		MessageEntry(sessionUser("old")),
		MessageEntry(sessionAssistant("old answer")),
		UsageEntry(0, 0, 0, 100_000, 100),
		MessageEntry(sessionUser("middle")),
		MessageEntry(sessionAssistant("middle answer")),
		UsageEntry(0, 0, 0, 110_000, 100),
		MessageEntry(sessionUser("latest")),
		MessageEntry(call),
		MessageEntry(sessionTool("call-1", "file contents")),
		MessageEntry(sessionAssistant("latest answer")),
		UsageEntry(0, 0, 0, 125_000, 100),
	}
	if tokens, ok := LatestContextTokens(entries); !ok || tokens != 125_100 {
		t.Fatalf("tokens: %d %v", tokens, ok)
	}
	retained, summarized := splitRetained(entries, DefaultContextOptions())
	if len(retained) != 4 {
		t.Fatalf("retained: %d", len(retained))
	}
	if retained[0].Content != "latest" || retained[1].ToolCalls[0].ID != "call-1" || retained[2].ToolCallID != "call-1" {
		t.Fatalf("retained: %+v", retained)
	}
	if summarized[len(summarized)-1].Content != "middle answer" {
		t.Fatalf("resumido: %+v", summarized)
	}
}

func TestCompactionResetsPersistedContextUsage(t *testing.T) {
	entries := []Entry{
		UsageEntry(0, 0, 0, 250_000, 500),
		CompactionEntry("summary", 250_500, 1, nil),
	}
	if _, ok := LatestContextTokens(entries); ok {
		t.Fatal("la compactacion deberia resetear el usage")
	}
}

func TestCompactedContextKeepsOriginalTaskAndWorkspaceState(t *testing.T) {
	entries := []Entry{
		MessageEntry(sessionUser("Fix compaction exactly")),
		MessageEntry(sessionCall("read-1", "read", `{"path":"src/session.rs"}`)),
		MessageEntry(sessionTool("read-1", "contents")),
		MessageEntry(sessionCall("bash-1", "bash", `{"command":"cargo test"}`)),
		MessageEntry(sessionTool("bash-1", "all tests passed")),
		CompactionEntry("summary", 100, 1, nil),
	}
	context := ContextMessages(entries)
	for _, want := range []string{"Fix compaction exactly", "Files read: src/session.rs", "cargo test => all tests passed"} {
		if !strings.Contains(context[0].Content, want) {
			t.Fatalf("falta %q en:\n%s", want, context[0].Content)
		}
	}
}

func TestContextOptionsDropOriginalTaskAndWorkspaceState(t *testing.T) {
	entries := []Entry{
		MessageEntry(sessionUser("Fix compaction exactly")),
		MessageEntry(sessionAssistant("done")),
		CompactionEntry("summary", 100, 1, nil),
	}
	context := ContextMessagesWith(entries, ContextOptions{})
	if !strings.Contains(context[0].Content, "summary") {
		t.Fatalf("got: %q", context[0].Content)
	}
	for _, unwanted := range []string{"Authoritative Original Task", "Authoritative Workspace State"} {
		if strings.Contains(context[0].Content, unwanted) {
			t.Fatalf("no deberia tener %q", unwanted)
		}
	}
}

func TestOldReadObservationsAreMasked(t *testing.T) {
	call := sessionCall("read-1", "read", `{"path":"large"}`)
	result := sessionTool("read-1", strings.Repeat("x", 5000))
	serialized := serializeConversation([]Message{call, result})
	if !strings.Contains(serialized, "bytes masked") {
		t.Fatalf("got: %q", serialized)
	}
	if len(serialized) >= 2000 {
		t.Fatalf("largo: %d", len(serialized))
	}
}

func TestCompactGeneratesStructuredSummary(t *testing.T) {
	provider := &summaryProvider{}
	entries := []Entry{}
	for range 30 {
		entries = append(entries, MessageEntry(sessionUser(strings.Repeat("x", 3800))))
	}
	summary, tokensBefore, retained, err := Compact(provider, "m1", entries)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(summary, "## Goal") {
		t.Fatalf("resumen: %q", summary[:40])
	}
	if tokensBefore != 0 {
		t.Fatalf("tokens_before: %d", tokensBefore)
	}
	if len(retained) == 0 || len(retained) >= len(entries) {
		t.Fatalf("retained: %d", len(retained))
	}
	last := provider.requests[len(provider.requests)-1]
	if !strings.Contains(last, "## Goal") || !strings.Contains(last, "## Critical Context") {
		t.Fatalf("prompt: %q", last[:200])
	}
}

func TestCompactFallsBackWhenTheModelIgnoresTheSchema(t *testing.T) {
	provider := &summaryProvider{content: "short summary"}
	entries := []Entry{}
	for range 30 {
		entries = append(entries, MessageEntry(sessionUser(strings.Repeat("x", 1000))))
	}
	summary, _, retained, err := Compact(provider, "small", entries)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(summary, "## Critical Context") || !strings.Contains(summary, "short summary") {
		t.Fatalf("resumen: %q", summary)
	}
	if len(retained) == 0 {
		t.Fatal("sin retained")
	}
	if len(provider.requests) != 2 {
		t.Fatalf("requests: %d", len(provider.requests))
	}
}

func TestCompactNeedsSomethingToSummarize(t *testing.T) {
	provider := &summaryProvider{}
	_, _, _, err := Compact(provider, "m", []Entry{})
	if err == nil {
		t.Fatal("esperaba error")
	}
	if err.Error() != "nothing to summarize" {
		t.Fatalf("error: %v", err)
	}
}

func TestRepeatedCompactionKeepsABoundedContext(t *testing.T) {
	provider := &summaryProvider{}
	entries := []Entry{MessageEntry(sessionUser("the original task must survive"))}
	for cycle := range 5 {
		for turn := range 10 {
			entries = append(entries, MessageEntry(sessionUser(strings.Repeat("x", 500)+string(rune('a'+turn)))))
		}
		summary, tokensBefore, retained, err := Compact(provider, "small", entries)
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, CompactionEntry(summary, tokensBefore, int64(cycle), retained))
		context := ContextMessages(entries)
		if !strings.Contains(context[0].Content, "the original task must survive") {
			t.Fatalf("ciclo %d: %q", cycle, context[0].Content[:80])
		}
		if len(context) > 3 {
			t.Fatalf("ciclo %d: %d mensajes", cycle, len(context))
		}
	}
}

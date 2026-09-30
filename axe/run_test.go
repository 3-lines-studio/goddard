package axe

import (
	"context"
	"strings"
	"sync"
	"testing"
)

type fakeProvider struct {
	responses []Response
	requests  [][]Message
}

func (f *fakeProvider) Complete(req *Request) (*Response, error) {
	messages := make([]Message, len(req.Messages))
	copy(messages, req.Messages)
	f.requests = append(f.requests, messages)
	if len(f.responses) == 0 {
		return nil, ProviderError("no fake response")
	}
	response := f.responses[0]
	f.responses = f.responses[1:]
	return &response, nil
}

func (f *fakeProvider) Stream(req *Request) *StreamHandle {
	events := make(chan StreamEvent, 16)
	response, err := f.Complete(req)
	if err != nil {
		close(events)
		return NewStreamHandle(events, func() (*Response, error) { return nil, err })
	}
	for _, call := range response.Message.ToolCalls {
		events <- StreamEvent{Kind: StreamToolCall, ToolCall: call}
	}
	if response.Message.Content != "" {
		events <- StreamEvent{Kind: StreamContent, Content: response.Message.Content}
	}
	events <- StreamEvent{
		Kind:        StreamTokens,
		Input:       response.Usage.Input,
		Output:      response.Usage.Output,
		CachedInput: response.Usage.CachedInput,
	}
	events <- StreamEvent{Kind: StreamDone}
	close(events)
	result := *response
	return NewStreamHandle(events, func() (*Response, error) { return &result, nil })
}

type flakyProvider struct {
	mutex     sync.Mutex
	attempts  int
	status    int
	failTimes int
}

func (f *flakyProvider) Complete(*Request) (*Response, error) {
	f.mutex.Lock()
	f.attempts++
	attempt := f.attempts
	f.mutex.Unlock()
	if attempt <= f.failTimes {
		return nil, HTTPError(f.status, "openai: flaky")
	}
	return &Response{Message: Message{Role: "assistant", Content: "ok"}}, nil
}

func (f *flakyProvider) Stream(req *Request) *StreamHandle {
	events := make(chan StreamEvent, 4)
	response, err := f.Complete(req)
	if err != nil {
		close(events)
		return NewStreamHandle(events, func() (*Response, error) { return nil, err })
	}
	events <- StreamEvent{Kind: StreamContent, Content: response.Message.Content}
	events <- StreamEvent{Kind: StreamDone}
	close(events)
	return NewStreamHandle(events, func() (*Response, error) { return response, nil })
}

func (f *flakyProvider) count() int {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	return f.attempts
}

type recorded struct {
	kind   string
	text   string
	call   ToolCall
	usage  Usage
	output ToolOutput
}

type testSink struct {
	SinkBase
	events   []recorded
	compact  bool
	steering []string
	polled   int
}

func (s *testSink) Assistant(turn int, message Message, usage Usage) {
	s.events = append(s.events, recorded{kind: "assistant", text: message.Content, usage: usage})
}

func (s *testSink) Tool(turn int, message Message) {
	s.events = append(s.events, recorded{kind: "tool", text: message.Content, usage: Usage{}})
}

func (s *testSink) ShouldCompact(int, int) bool {
	return s.compact
}

func (s *testSink) PendingUserInput() string {
	if s.steering == nil {
		return ""
	}
	s.polled++
	if s.polled <= len(s.steering) {
		return s.steering[s.polled-1]
	}
	return ""
}

type emptyArgs struct{}

func callTool(id, name, args string) Message {
	return Message{Role: "assistant", ToolCalls: []ToolCall{{ID: id, Name: name, Arguments: args}}}
}

func userMessageOf(content string) Message {
	return Message{Role: "user", Content: content}
}

func assistantMessage(content string) Message {
	return Message{Role: "assistant", Content: content}
}

func runOptions(tools []Tool, maxTurns int) *RunOptions {
	return &RunOptions{Model: "m", Tools: tools, MaxTurns: maxTurns}
}

func TestRunRetries429ThenSucceeds(t *testing.T) {
	provider := &flakyProvider{status: 429, failTimes: 2}
	end := RunStream(context.Background(), provider, runOptions(nil, 5), []Message{userMessageOf("go")}, &testSink{})
	if end.Outcome.Kind != OutcomeDone {
		t.Fatalf("outcome: %+v", end.Outcome)
	}
	if len(end.Messages) != 2 || end.Messages[1].Content != "ok" {
		t.Fatalf("mensajes: %+v", end.Messages)
	}
	if provider.count() != 3 {
		t.Fatalf("intentos: %d", provider.count())
	}
}

func TestRunDoesNotRetry400(t *testing.T) {
	provider := &flakyProvider{status: 400, failTimes: 10}
	end := RunStream(context.Background(), provider, runOptions(nil, 5), []Message{userMessageOf("go")}, &testSink{})
	if end.Outcome.Kind != OutcomeFailed || !strings.Contains(end.Outcome.Failure, "flaky") {
		t.Fatalf("outcome: %+v", end.Outcome)
	}
	if provider.count() != 1 {
		t.Fatalf("intentos: %d", provider.count())
	}
}

func TestRunExecutesToolsAndReturnsTranscript(t *testing.T) {
	upper := NewTool("upper", "uppercase", "{}", func(args struct {
		S string `json:"s"`
	}) string {
		return strings.ToUpper(args.S)
	})
	provider := &fakeProvider{responses: []Response{
		{Message: callTool("c1", "upper", `{"s":"hi"}`), Usage: Usage{Input: 10, Output: 5}},
		{Message: assistantMessage("done"), Usage: Usage{Input: 20, Output: 2}},
	}}
	input := []Message{userMessageOf("go")}
	sink := &testSink{}
	end := RunStream(context.Background(), provider, runOptions([]Tool{upper}, 5), input, sink)

	if len(input) != 1 || input[0].Content != "go" {
		t.Fatalf("el input se muto: %+v", input)
	}
	if len(end.Messages) != 4 {
		t.Fatalf("mensajes: %+v", end.Messages)
	}
	if end.Messages[2].Role != "tool" || end.Messages[2].ToolCallID != "c1" || end.Messages[2].Content != "HI" {
		t.Fatalf("resultado: %+v", end.Messages[2])
	}
	if end.Messages[3].Content != "done" {
		t.Fatalf("final: %+v", end.Messages[3])
	}
	if len(sink.events) != 3 {
		t.Fatalf("eventos: %+v", sink.events)
	}
	if sink.events[0].usage.Input != 10 || sink.events[0].usage.Output != 5 {
		t.Fatalf("usage 0: %+v", sink.events[0])
	}
	if sink.events[1].usage != (Usage{}) {
		t.Fatalf("usage 1: %+v", sink.events[1])
	}
	if sink.events[2].usage.Input != 20 {
		t.Fatalf("usage 2: %+v", sink.events[2])
	}
	last := provider.requests[len(provider.requests)-1]
	if len(last) != 3 || last[2].Content != "HI" {
		t.Fatalf("ultimo request: %+v", last)
	}
}

func TestRunRedactsSecretsFromToolOutputAndUserInput(t *testing.T) {
	secret := "sk-abcdefghijklmnopqrstuvwx"
	leak := NewTool("leak", "returns a secret", "{}", func(emptyArgs) string {
		return "PASSWORD=" + secret + "\nconnect postgres://user:pass@db/x"
	})
	provider := &fakeProvider{responses: []Response{
		{Message: callTool("c1", "leak", "{}")},
		{Message: assistantMessage("done")},
	}}
	end := RunStream(context.Background(), provider, runOptions([]Tool{leak}, 5),
		[]Message{userMessageOf("my key is " + secret)}, &testSink{})

	for _, message := range end.Messages {
		if strings.Contains(message.Content, secret) {
			t.Fatalf("el secreto quedo en el transcript: %+v", message)
		}
		if strings.Contains(message.Content, "user:pass") {
			t.Fatalf("la credencial quedo en el transcript: %+v", message)
		}
	}
	if !strings.Contains(end.Messages[2].Content, "PASSWORD=[REDACTED]") {
		t.Fatalf("resultado: %q", end.Messages[2].Content)
	}
	if !strings.Contains(end.Messages[2].Content, "postgres://[REDACTED]@db/x") {
		t.Fatalf("url: %q", end.Messages[2].Content)
	}
	last := provider.requests[len(provider.requests)-1]
	if strings.Contains(last[0].Content, secret) || strings.Contains(last[2].Content, secret) {
		t.Fatalf("el secreto llego al proveedor: %+v", last)
	}
}

func TestRunContinuesOnToolError(t *testing.T) {
	boom := NewTool("boom", "always fails", "{}", func(emptyArgs) string { return "error: boom" })
	provider := &fakeProvider{responses: []Response{
		{Message: callTool("c1", "boom", "{}")},
		{Message: assistantMessage("recovered")},
	}}
	end := RunStream(context.Background(), provider, runOptions([]Tool{boom}, 5), []Message{userMessageOf("go")}, &testSink{})
	if end.Messages[2].Content != "error: boom" || end.Messages[3].Content != "recovered" {
		t.Fatalf("mensajes: %+v", end.Messages)
	}
}

func TestRunUnknownTool(t *testing.T) {
	provider := &fakeProvider{responses: []Response{
		{Message: callTool("c1", "nope", "{}")},
		{Message: assistantMessage("ok")},
	}}
	end := RunStream(context.Background(), provider, runOptions(nil, 5), []Message{userMessageOf("go")}, &testSink{})
	if !strings.Contains(end.Messages[2].Content, "unknown tool") {
		t.Fatalf("got: %q", end.Messages[2].Content)
	}
}

func TestRunMaxTurns(t *testing.T) {
	responses := make([]Response, 10)
	for i := range responses {
		responses[i] = Response{Message: callTool("c", "x", "{}")}
	}
	provider := &fakeProvider{responses: responses}
	end := RunStream(context.Background(), provider, runOptions(nil, 3), []Message{userMessageOf("go")}, &testSink{})
	if end.Outcome.Kind != OutcomeMaxTurns {
		t.Fatalf("outcome: %+v", end.Outcome)
	}
	if len(end.Messages) != 1+3*2 {
		t.Fatalf("mensajes: %d", len(end.Messages))
	}
}

func TestRunEndSeparatesLastContextUsageFromTotals(t *testing.T) {
	noop := NewTool("noop", "noop", "{}", func(emptyArgs) string { return "ok" })
	provider := &fakeProvider{responses: []Response{
		{Message: callTool("c1", "noop", "{}"), Usage: Usage{Input: 10, Output: 5, CachedInput: 3}, StopReason: "tool_calls"},
		{Message: assistantMessage("done"), Usage: Usage{Input: 20, Output: 8, CachedInput: 7}, StopReason: "stop"},
	}}
	end := RunStream(context.Background(), provider, runOptions([]Tool{noop}, 10), []Message{userMessageOf("hi")}, &testSink{})
	if end.Outcome.Kind != OutcomeDone {
		t.Fatalf("outcome: %+v", end.Outcome)
	}
	if end.Usage != (Usage{Input: 30, Output: 13, CachedInput: 10}) {
		t.Fatalf("usage: %+v", end.Usage)
	}
	if end.Context != (Usage{Input: 20, Output: 8, CachedInput: 7}) {
		t.Fatalf("context: %+v", end.Context)
	}
}

func TestRunLengthStopDoesNotExecuteToolCalls(t *testing.T) {
	ran := false
	boom := NewTool("boom", "records execution", "{}", func(emptyArgs) string {
		ran = true
		return "executed"
	})
	provider := &fakeProvider{responses: []Response{
		{Message: callTool("c1", "boom", `{"truncated":"args"}`), StopReason: "length"},
		{Message: assistantMessage("recovered")},
	}}
	end := RunStream(context.Background(), provider, runOptions([]Tool{boom}, 5), []Message{userMessageOf("go")}, &testSink{})
	if ran {
		t.Fatal("la tool corrio con argumentos truncados")
	}
	if !strings.Contains(end.Messages[2].Content, "not executed") || !strings.Contains(end.Messages[2].Content, "truncated") {
		t.Fatalf("got: %q", end.Messages[2].Content)
	}
	if end.Messages[3].Content != "recovered" {
		t.Fatalf("final: %q", end.Messages[3].Content)
	}
}

func TestRunParallelToolsKeepOrder(t *testing.T) {
	provider := &fakeProvider{responses: []Response{
		{Message: Message{Role: "assistant", ToolCalls: []ToolCall{
			{ID: "c1", Name: "t1", Arguments: "{}"},
			{ID: "c2", Name: "t2", Arguments: "{}"},
			{ID: "c3", Name: "t3", Arguments: "{}"},
		}}},
		{Message: assistantMessage("done")},
	}}
	tools := []Tool{
		NewTool("t1", "", "{}", func(emptyArgs) string { return "R1" }),
		NewTool("t2", "", "{}", func(emptyArgs) string { return "R2" }),
		NewTool("t3", "", "{}", func(emptyArgs) string { return "R3" }),
	}
	end := RunStream(context.Background(), provider, runOptions(tools, 5), []Message{userMessageOf("go")}, &testSink{})
	want := []string{"R1", "R2", "R3"}
	for index, content := range want {
		message := end.Messages[2+index]
		if message.Content != content || message.ToolCallID != []string{"c1", "c2", "c3"}[index] {
			t.Fatalf("posicion %d: %+v", index, message)
		}
	}
	if end.Messages[5].Content != "done" {
		t.Fatalf("final: %+v", end.Messages[5])
	}
}

func TestRunInjectsSteeringBetweenTurns(t *testing.T) {
	provider := &fakeProvider{responses: []Response{
		{Message: assistantMessage("first")},
		{Message: assistantMessage("second")},
	}}
	sink := &testSink{steering: []string{"steer one", "steer two"}}
	end := RunStream(context.Background(), provider, runOptions(nil, 5), []Message{userMessageOf("go")}, sink)

	if end.Outcome.Kind != OutcomeDone {
		t.Fatalf("outcome: %+v", end.Outcome)
	}
	want := []struct {
		role    string
		content string
	}{
		{"user", "go"},
		{"user", "steer one"},
		{"assistant", "first"},
		{"user", "steer two"},
		{"assistant", "second"},
	}
	if len(end.Messages) != len(want) {
		t.Fatalf("mensajes: %+v", end.Messages)
	}
	for index, expected := range want {
		if end.Messages[index].Role != expected.role || end.Messages[index].Content != expected.content {
			t.Fatalf("posicion %d: %+v", index, end.Messages[index])
		}
	}
}

func TestRunCompactOutcomeLeavesTranscript(t *testing.T) {
	echo := NewTool("echo", "echoes", "{}", func(emptyArgs) string { return "ok" })
	provider := &fakeProvider{responses: []Response{
		{Message: callTool("c1", "echo", "{}"), Usage: Usage{Input: 20, Output: 2}},
		{Message: assistantMessage("should not run")},
	}}
	sink := &testSink{compact: true}
	end := RunStream(context.Background(), provider, runOptions([]Tool{echo}, 8), []Message{userMessageOf("go")}, sink)
	if end.Outcome.Kind != OutcomeCompact {
		t.Fatalf("outcome: %+v", end.Outcome)
	}
	if len(end.Messages) != 3 || end.Messages[2].Role != "tool" {
		t.Fatalf("mensajes: %+v", end.Messages)
	}
	if len(provider.requests) != 1 {
		t.Fatalf("requests: %d", len(provider.requests))
	}
}

func TestRunDoesNotRetryAfterEventsWereEmitted(t *testing.T) {
	provider := &halfProvider{}
	end := RunStream(context.Background(), provider, runOptions(nil, 5), []Message{userMessageOf("go")}, &testSink{})
	if end.Outcome.Kind != OutcomeFailed {
		t.Fatalf("outcome: %+v", end.Outcome)
	}
	if provider.count() != 1 {
		t.Fatalf("intentos: %d", provider.count())
	}
}

type halfProvider struct {
	mutex    sync.Mutex
	attempts int
}

func (h *halfProvider) Complete(*Request) (*Response, error) {
	return nil, ProviderError("unused")
}

func (h *halfProvider) Stream(*Request) *StreamHandle {
	h.mutex.Lock()
	h.attempts++
	h.mutex.Unlock()
	events := make(chan StreamEvent, 4)
	events <- StreamEvent{Kind: StreamContent, Content: "half"}
	close(events)
	return NewStreamHandle(events, func() (*Response, error) {
		return nil, TransportError("connection reset")
	})
}

func (h *halfProvider) count() int {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	return h.attempts
}

func TestRunCancelMidBatchSynthesizesResults(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	first := NewTool("first", "", "{}", func(emptyArgs) string {
		cancel()
		return "done"
	})
	first.Sequential = true
	second := NewTool("second", "", "{}", func(emptyArgs) string { return "should not run" })
	second.Sequential = true

	provider := &fakeProvider{responses: []Response{
		{Message: Message{Role: "assistant", ToolCalls: []ToolCall{
			{ID: "c1", Name: "first", Arguments: "{}"},
			{ID: "c2", Name: "second", Arguments: "{}"},
		}}},
	}}
	end := RunStream(ctx, provider, runOptions([]Tool{first, second}, 5), []Message{userMessageOf("go")}, &testSink{})
	if end.Outcome.Kind != OutcomeCancelled {
		t.Fatalf("outcome: %+v", end.Outcome)
	}
	if end.Messages[2].Content != "done" {
		t.Fatalf("primera: %q", end.Messages[2].Content)
	}
	if !strings.Contains(end.Messages[3].Content, "not executed") {
		t.Fatalf("segunda: %q", end.Messages[3].Content)
	}
}

func TestRunCompactsAfterCompleteToolBatch(t *testing.T) {
	noop := NewTool("noop", "noop", "{}", func(emptyArgs) string { return "ok" })
	provider := &fakeProvider{responses: []Response{
		{Message: callTool("c1", "noop", "{}"), Usage: Usage{Input: 100, Output: 5}},
	}}
	sink := &testSink{compact: true}
	end := RunStream(context.Background(), provider, runOptions([]Tool{noop}, 5), []Message{userMessageOf("go")}, sink)
	if end.Outcome.Kind != OutcomeCompact {
		t.Fatalf("outcome: %+v", end.Outcome)
	}
	if len(end.Messages) != 3 {
		t.Fatalf("mensajes: %+v", end.Messages)
	}
	if end.Messages[2].Content != "ok" {
		t.Fatalf("resultado: %+v", end.Messages[2])
	}
}

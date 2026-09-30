package axe

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sse(events ...string) string {
	var out strings.Builder
	for _, event := range events {
		out.WriteString("data: " + event + "\n\n")
	}
	out.WriteString("data: [DONE]\n\n")
	return out.String()
}

func TestEndToEndRunAgainstAServer(t *testing.T) {
	workdir := t.TempDir()
	secret := "PASSWORD=hunter2xyz"

	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("body: %v", err)
		}
		requests = append(requests, body)
		w.Header().Set("Content-Type", "text/event-stream")

		if len(requests) == 1 {
			if body["stream"] != true {
				t.Errorf("el primer request no pide stream: %v", body["stream"])
			}
			tools := body["tools"].([]any)
			if len(tools) != 6 {
				t.Errorf("tools: %d", len(tools))
			}
			name := tools[3].(map[string]any)["function"].(map[string]any)["name"]
			if name != "bash" {
				t.Errorf("cuarta tool: %v", name)
			}
			w.Write([]byte(sse(
				`{"choices":[{"delta":{"reasoning_content":"pienso "}}]}`,
				`{"choices":[{"delta":{"content":"voy a correr bash"}}]}`,
				`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","function":{"name":"bash","arguments":"{\"command\":\"echo hola > out.txt; cat secret.txt\"}"}}]}}]}`,
				`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
				`{"choices":[],"usage":{"prompt_tokens":11,"completion_tokens":3,"prompt_tokens_details":{"cached_tokens":2}}}`,
			)))
			return
		}
		w.Write([]byte(sse(
			`{"choices":[{"delta":{"content":"listo"}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
			`{"choices":[],"usage":{"prompt_tokens":21,"completion_tokens":1}}`,
		)))
	}))
	defer server.Close()

	seedSecret(t, workdir, secret)

	provider := NewOpenAI(server.URL, "k1")
	tools := BuildTools(workdir)
	options := &RunOptions{
		Model:    "m1",
		System:   SystemPrompt(tools),
		Tools:    tools,
		MaxTurns: 4,
	}
	sink := &testSink{}
	end := RunStream(context.Background(), provider, options,
		[]Message{{Role: "user", Content: "crea out.txt y conta el secreto"}}, sink)

	if end.Outcome.Kind != OutcomeDone {
		t.Fatalf("outcome: %+v (%s)", end.Outcome, end.Outcome.Failure)
	}
	if end.Usage != (Usage{Input: 32, Output: 4, CachedInput: 2}) {
		t.Fatalf("usage: %+v", end.Usage)
	}
	if end.Context != (Usage{Input: 21, Output: 1}) {
		t.Fatalf("context: %+v", end.Context)
	}

	if content, err := os.ReadFile(filepath.Join(workdir, "out.txt")); err != nil || string(content) != "hola\n" {
		t.Fatalf("el bash no escribio el archivo: %q %v", content, err)
	}

	if len(end.Messages) != 4 {
		t.Fatalf("mensajes: %d", len(end.Messages))
	}
	if end.Messages[0].Role != "user" || end.Messages[1].Role != "assistant" {
		t.Fatalf("mensajes: %+v", end.Messages)
	}
	if end.Messages[2].Role != "tool" || end.Messages[2].ToolCallID != "c1" {
		t.Fatalf("resultado: %+v", end.Messages[2])
	}
	if end.Messages[3].Content != "listo" {
		t.Fatalf("final: %+v", end.Messages[3])
	}
	if !strings.Contains(end.Messages[1].Content, "voy a correr bash") || end.Messages[1].Reasoning != "pienso " {
		t.Fatalf("assistant: %+v", end.Messages[1])
	}

	if len(requests) != 2 {
		t.Fatalf("requests: %d", len(requests))
	}
	second := requests[1]["messages"].([]any)
	if second[0].(map[string]any)["role"] != "system" {
		t.Fatalf("el system no encabeza: %v", second[0])
	}
	toolResult := lastRole(second, "tool")
	if toolResult == nil {
		t.Fatal("el resultado del tool no llego al segundo request")
	}
	if toolResult["tool_call_id"] != "c1" {
		t.Fatalf("tool_call_id: %v", toolResult)
	}
	if !strings.Contains(toolResult["content"].(string), "[REDACTED]") {
		t.Fatalf("el resultado no llego al modelo o no se redacto: %q", toolResult["content"])
	}

	for index, request := range requests {
		encoded, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "hunter2xyz") {
			t.Fatalf("el secreto llego al proveedor en el request %d", index+1)
		}
	}
	if !strings.Contains(end.Messages[2].Content, "[REDACTED]") {
		t.Fatalf("el transcript no redacto: %q", end.Messages[2].Content)
	}
}

func TestEndToEndSecondRequestCarriesTheToolResult(t *testing.T) {
	workdir := t.TempDir()
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		requests = append(requests, body)
		w.Header().Set("Content-Type", "text/event-stream")
		if len(requests) == 1 {
			w.Write([]byte(sse(
				`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","function":{"name":"write","arguments":"{\"path\":\"nota.txt\",\"content\":\"hola\\n\"}"}}]}}]}`,
				`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
			)))
			return
		}
		w.Write([]byte(sse(`{"choices":[{"delta":{"content":"ok"}}]}`)))
	}))
	defer server.Close()

	provider := NewOpenAI(server.URL, "k1")
	tools := BuildTools(workdir)
	end := RunStream(context.Background(), provider,
		&RunOptions{Model: "m1", Tools: tools, MaxTurns: 3},
		[]Message{{Role: "user", Content: "escribi nota.txt"}}, &SinkBase{})

	if end.Outcome.Kind != OutcomeDone {
		t.Fatalf("outcome: %+v", end.Outcome)
	}
	if content, err := os.ReadFile(filepath.Join(workdir, "nota.txt")); err != nil || string(content) != "hola\n" {
		t.Fatalf("la tool write no escribio: %q %v", content, err)
	}
	if end.Messages[2].Content != "wrote nota.txt (5 bytes)" {
		t.Fatalf("resultado: %q", end.Messages[2].Content)
	}
	second := requests[1]["messages"].([]any)
	toolResult := lastRole(second, "tool")
	if toolResult == nil {
		t.Fatal("el resultado del tool no viajo")
	}
	if !strings.Contains(toolResult["content"].(string), "wrote nota.txt") {
		t.Fatalf("el resultado no viajo: %v", toolResult)
	}
}

func lastRole(messages []any, role string) map[string]any {
	for index := len(messages) - 1; index >= 0; index-- {
		entry := messages[index].(map[string]any)
		if entry["role"] == role {
			return entry
		}
	}
	return nil
}

func TestEndToEndServerErrorEndsTheRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"Invalid API key"}}`))
	}))
	defer server.Close()

	provider := NewOpenAI(server.URL, "k1")
	end := RunStream(context.Background(), provider,
		&RunOptions{Model: "m1", MaxTurns: 3},
		[]Message{{Role: "user", Content: "hola"}}, &SinkBase{})

	if end.Outcome.Kind != OutcomeFailed {
		t.Fatalf("outcome: %+v", end.Outcome)
	}
	if !strings.Contains(end.Outcome.Failure, "openai: 401") {
		t.Fatalf("failure: %q", end.Outcome.Failure)
	}
	if len(end.Messages) != 1 {
		t.Fatalf("mensajes: %+v", end.Messages)
	}
}

func seedSecret(t *testing.T, workdir, secret string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(workdir, "secret.txt"), []byte(secret+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

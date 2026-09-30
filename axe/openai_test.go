package axe

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func openAITestRequest() *Request {
	return &Request{
		Model:  "m1",
		System: "be brief",
		Messages: []Message{
			{
				Role:    "user",
				Content: "go",
				Images:  []Image{{Path: "p.png", URL: "data:image/png;base64,AAAA"}},
			},
			{
				Role:      "assistant",
				ToolCalls: []ToolCall{{ID: "c9", Name: "read", Arguments: `{"path":"a.txt"}`}},
				Reasoning: "because",
			},
			{
				Role:       "tool",
				Content:    "hi",
				ToolCallID: "c9",
				Images:     []Image{{Path: "shot.png", URL: "data:image/png;base64,BBBB"}},
			},
			{Role: "tool", ToolCallID: "c11"},
			{Role: "assistant", Content: "plain"},
			{Role: "user"},
		},
		Tools: []Tool{
			{Name: "read", Description: "d", Parameters: map[string]any{"x": 1}},
			{Name: "bash", Description: "b", Parameters: map[string]any{"y": 2}},
		},
	}
}

func openAIEmptyRequest() *Request {
	full := openAITestRequest()
	return &Request{
		Model:    "m2",
		Messages: full.Messages[5:],
	}
}

func TestParidadOpenAIConRust(t *testing.T) {
	expected := readTestdata(t, "testdata/paridad-openai.txt")
	provider := NewOpenAI("http://ignored", "k1")

	cases := map[string]func() string{
		"url": func() string {
			url, _, _, err := provider.buildRequest(openAITestRequest(), true)
			if err != nil {
				t.Fatal(err)
			}
			return url
		},
		"body_stream": func() string {
			_, _, body, err := provider.buildRequest(openAITestRequest(), true)
			if err != nil {
				t.Fatal(err)
			}
			return escapeEdit(string(body))
		},
		"body_plain": func() string {
			_, _, body, err := provider.buildRequest(openAITestRequest(), false)
			if err != nil {
				t.Fatal(err)
			}
			return escapeEdit(string(body))
		},
		"body_empty": func() string {
			_, _, body, err := provider.buildRequest(openAIEmptyRequest(), false)
			if err != nil {
				t.Fatal(err)
			}
			return escapeEdit(string(body))
		},
		"headers": func() string {
			_, headers, _, err := provider.buildRequest(openAITestRequest(), true)
			if err != nil {
				t.Fatal(err)
			}
			return fmt.Sprintf(`[("Content-Type", "%s"), ("Authorization", "%s")]`,
				headers["Content-Type"], headers["Authorization"])
		},
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

func TestOpenAIConversationRoundTrip(t *testing.T) {
	request := openAITestRequest()
	var seen map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer k1" {
			t.Errorf("authorization: %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("content-type: %q", got)
		}
		json.NewDecoder(r.Body).Decode(&seen)
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"ok","reasoning_content":"thought","tool_calls":[{"id":"c10","function":{"name":"read","arguments":"{\"path\":\"b.txt\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":123,"completion_tokens":7,"prompt_tokens_details":{"cached_tokens":100}}}`)
	}))
	defer server.Close()

	provider := NewOpenAI(server.URL, "k1")
	request.Context = context.Background()
	response, err := provider.Complete(request)
	if err != nil {
		t.Fatal(err)
	}

	messages := seen["messages"].([]any)
	if len(messages) != 7 {
		t.Fatalf("mensajes: %d", len(messages))
	}
	if seen["model"] != "m1" {
		t.Errorf("model: %v", seen["model"])
	}
	first := messages[0].(map[string]any)
	if first["role"] != "system" || first["content"] != "be brief" {
		t.Errorf("system: %v", first)
	}
	blocks := messages[1].(map[string]any)["content"].([]any)
	if blocks[0].(map[string]any)["type"] != "text" || blocks[1].(map[string]any)["type"] != "image_url" {
		t.Errorf("bloques: %v", blocks)
	}
	assistant := messages[2].(map[string]any)
	if assistant["reasoning_content"] != "because" {
		t.Errorf("reasoning: %v", assistant)
	}
	calls := assistant["tool_calls"].([]any)
	if calls[0].(map[string]any)["id"] != "c9" {
		t.Errorf("tool calls: %v", calls)
	}
	if _, present := seen["stream"]; present {
		t.Errorf("stream no deberia estar: %v", seen["stream"])
	}

	if response.Message.Content != "ok" {
		t.Errorf("content: %q", response.Message.Content)
	}
	if response.Message.Reasoning != "thought" {
		t.Errorf("reasoning: %q", response.Message.Reasoning)
	}
	if len(response.Message.ToolCalls) != 1 || response.Message.ToolCalls[0].ID != "c10" {
		t.Errorf("tool calls: %+v", response.Message.ToolCalls)
	}
	if response.Message.ToolCalls[0].Arguments != `{"path":"b.txt"}` {
		t.Errorf("arguments: %q", response.Message.ToolCalls[0].Arguments)
	}
	if response.Usage.Input != 123 || response.Usage.Output != 7 || response.Usage.CachedInput != 100 {
		t.Errorf("usage: %+v", response.Usage)
	}
	if response.StopReason != "tool_calls" {
		t.Errorf("stop: %q", response.StopReason)
	}
}

func TestOpenAIStreamRoundTrip(t *testing.T) {
	chunks := []string{
		"data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"think \"}}]}\n\n",
		"data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"hard\"}}]}\n\n",
		"data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n",
		"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"c1\",\"function\":{\"name\":\"read\",\"arguments\":\"{\\\"path\\\":\"}}]}}]}\n\n",
		"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"\\\"a\\\"}\"}}]}}]}\n\n",
		"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"length\"}]}\n\n",
		"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":2,\"prompt_tokens_details\":{\"cached_tokens\":3}}}\n\n",
		"data: [DONE]\n\n",
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for _, chunk := range chunks {
			fmt.Fprint(w, chunk)
			flusher.Flush()
		}
	}))
	defer server.Close()

	provider := NewOpenAI(server.URL, "k1")
	request := &Request{Context: context.Background(), Model: "m1", Messages: []Message{{Role: "user", Content: "go"}}}
	handle := provider.Stream(request)

	var events []StreamEvent
	for event := range handle.Events {
		events = append(events, event)
	}
	response, err := handle.Join()
	if err != nil {
		t.Fatal(err)
	}

	if response.Message.Content != "hi" {
		t.Errorf("content: %q", response.Message.Content)
	}
	if response.Message.Reasoning != "think hard" {
		t.Errorf("reasoning: %q", response.Message.Reasoning)
	}
	if len(response.Message.ToolCalls) != 1 || response.Message.ToolCalls[0].Arguments != `{"path":"a"}` {
		t.Errorf("tool calls: %+v", response.Message.ToolCalls)
	}
	if response.Usage.Input != 4 || response.Usage.Output != 2 || response.Usage.CachedInput != 3 {
		t.Errorf("usage: %+v", response.Usage)
	}
	if response.StopReason != "length" {
		t.Errorf("stop: %q", response.StopReason)
	}
	hasDone := false
	hasCall := false
	for _, event := range events {
		if event.Kind == StreamDone {
			hasDone = true
		}
		if event.Kind == StreamToolCall && event.ToolCall.Arguments == `{"path":"a"}` {
			hasCall = true
		}
	}
	if !hasDone || !hasCall {
		t.Errorf("eventos: %+v", events)
	}
}

func TestOpenAIErrorStatusKeepsTheMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"Invalid API key"}}`)
	}))
	defer server.Close()

	provider := NewOpenAI(server.URL, "k1")
	_, err := provider.Complete(&Request{Context: context.Background(), Model: "m1"})
	if err == nil {
		t.Fatal("esperaba error")
	}
	message := err.Error()
	if !strings.Contains(message, "openai: 401") || !strings.Contains(message, "Invalid API key") {
		t.Errorf("got: %q", message)
	}

	failure, ok := err.(*Error)
	if !ok || failure.Kind != ErrHTTP || failure.Status != 401 {
		t.Errorf("tipo: %#v", err)
	}
	if retryableError(err) {
		t.Error("un 401 no es retryable")
	}
}

func TestOpenAIStreamErrorKeepsTheMessage(t *testing.T) {
	body := fmt.Sprintf(`{"error":{"message":".messages[58].image[0]: %s unsupported image"}}`, strings.Repeat("x", 200))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, body)
	}))
	defer server.Close()

	provider := NewOpenAI(server.URL, "k1")
	handle := provider.Stream(&Request{Context: context.Background(), Model: "m1"})
	for range handle.Events {
	}
	_, err := handle.Join()
	if err == nil {
		t.Fatal("esperaba error")
	}
	if !strings.Contains(err.Error(), "openai: 400") || !strings.Contains(err.Error(), "unsupported image") {
		t.Errorf("got: %q", err.Error())
	}
}

func TestOpenAIEmptyAssistantKeepsContent(t *testing.T) {
	var seen map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&seen)
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"prompt_tokens_details":{"cached_tokens":0}}}`)
	}))
	defer server.Close()

	provider := NewOpenAI(server.URL, "k1")
	request := &Request{
		Context: context.Background(),
		Model:   "m1",
		Messages: []Message{
			{Role: "user", Content: "hi"},
			{Role: "assistant"},
		},
	}
	if _, err := provider.Complete(request); err != nil {
		t.Fatal(err)
	}
	messages := seen["messages"].([]any)
	assistant := messages[1].(map[string]any)
	if assistant["role"] != "assistant" || assistant["content"] != "" {
		t.Errorf("assistant: %v", assistant)
	}
	if _, present := assistant["tool_calls"]; present {
		t.Errorf("tool_calls no deberia estar: %v", assistant)
	}
}

func TestOpenAIRejectsABodyWithoutEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "<html>502 Bad Gateway</html>")
	}))
	defer server.Close()

	provider := NewOpenAI(server.URL, "k1")
	handle := provider.Stream(&Request{Context: context.Background(), Model: "m1"})
	for range handle.Events {
	}
	_, err := handle.Join()
	if err == nil || !strings.Contains(err.Error(), "invalid response body") {
		t.Fatalf("got: %v", err)
	}
}

func TestStreamAccParsesQuirkyEvents(t *testing.T) {
	content := func(events []StreamEvent) string {
		for _, event := range events {
			if event.Kind == StreamContent {
				return event.Content
			}
		}
		return ""
	}
	cases := []struct {
		name   string
		chunks []string
		want   string
	}{
		{"crlf", []string{"data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\r\n\r\n"}, "hi"},
		{
			"crlf partido",
			[]string{"data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\r", "\n\r\n"},
			"hi",
		},
		{
			"multiples data",
			[]string{"data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}\ndata: ]}\n\n"},
			"x",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			acc := &streamAcc{}
			var events []StreamEvent
			for _, chunk := range test.chunks {
				acc.feed([]byte(chunk), func(event StreamEvent) { events = append(events, event) }, true)
			}
			if got := content(events); got != test.want {
				t.Errorf("got: %q, want %q", got, test.want)
			}
		})
	}
}

func TestStreamAccBoundsRawAndKeepsLargeEvents(t *testing.T) {
	acc := &streamAcc{}
	acc.feed([]byte(strings.Repeat("x", streamRawLimit+1)), nil, true)
	if len(acc.raw) != streamRawLimit {
		t.Errorf("raw: %d", len(acc.raw))
	}
	if acc.events != 0 {
		t.Errorf("eventos: %d", acc.events)
	}

	acc = &streamAcc{}
	var events []StreamEvent
	big := strings.Repeat("x", 1024*1024)
	event := fmt.Sprintf("data: {\"choices\":[{\"delta\":{\"content\":\"%s\"}}]}\n\n", big)
	split := len(event) - 2
	acc.feed([]byte(event[:split]), func(e StreamEvent) { events = append(events, e) }, true)
	acc.feed([]byte(event[split:]), func(e StreamEvent) { events = append(events, e) }, true)
	if len(events) == 0 || len(events[0].Content) != len(big) {
		t.Fatalf("eventos: %d", len(events))
	}
	if len(acc.raw) != streamRawLimit {
		t.Errorf("raw: %d", len(acc.raw))
	}
}

func TestStreamAccRecordsProviderErrors(t *testing.T) {
	acc := &streamAcc{}
	acc.feed([]byte("data: {\"error\":{\"message\":\"provider failed\",\"code\":502},\"choices\":[{\"finish_reason\":\"error\",\"delta\":{}}]}\n\n"), nil, true)
	if acc.streamError == nil || *acc.streamError != "provider failed" {
		t.Fatalf("stream error: %v", acc.streamError)
	}
}

func TestOpenAIKeepsTheWholeErrorBody(t *testing.T) {
	acc := &streamAcc{}
	body := fmt.Sprintf(`{"error":{"message":"%s"}}`, strings.Repeat("x", 300))
	acc.feed([]byte(body), nil, true)
	if string(acc.raw) != body {
		t.Fatalf("raw: %q", acc.raw)
	}
	if acc.events != 0 {
		t.Fatalf("eventos: %d", acc.events)
	}
}

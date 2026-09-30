package axe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	maxStreamToolCalls = 64
	streamRawLimit     = 64 * 1024
	connectTimeout     = 10 * time.Second
	idleReadTimeout    = 60 * time.Second
)

type OpenAI struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewOpenAI(baseURL, apiKey string) *OpenAI {
	transport := &http.Transport{
		DialContext:        (&net.Dialer{Timeout: connectTimeout}).DialContext,
		DisableCompression: true,
		MaxIdleConns:       4,
		IdleConnTimeout:    90 * time.Second,
	}
	return &OpenAI{baseURL: baseURL, apiKey: apiKey, client: &http.Client{Transport: transport}}
}

func (o *OpenAI) Complete(req *Request) (*Response, error) {
	url, headers, body, err := o.buildRequest(req, false)
	if err != nil {
		return nil, err
	}
	return o.runRequest(req.Context, url, headers, body, false, nil)
}

func (o *OpenAI) Stream(req *Request) *StreamHandle {
	events := make(chan StreamEvent, 64)
	done := make(chan struct{})
	var response *Response
	var streamErr error
	go func() {
		defer close(events)
		defer close(done)
		url, headers, body, err := o.buildRequest(req, true)
		if err != nil {
			streamErr = err
			return
		}
		response, streamErr = o.runRequest(req.Context, url, headers, body, true, func(event StreamEvent) {
			events <- event
		})
	}()
	return NewStreamHandle(events, func() (*Response, error) {
		for range events {
		}
		<-done
		return response, streamErr
	})
}

func (o *OpenAI) buildRequest(req *Request, stream bool) (string, map[string]string, []byte, error) {
	messages := make([]oaRequestMessage, 0, len(req.Messages)+1)
	if req.System != "" {
		messages = append(messages, oaRequestMessage{Role: "system", Content: req.System})
	}
	for _, message := range req.Messages {
		entry := oaRequestMessage{
			Role:    message.Role,
			Content: messageContent(message),
		}
		entry.ReasoningContent = message.Reasoning
		entry.ToolCallID = message.ToolCallID
		for _, call := range message.ToolCalls {
			entry.ToolCalls = append(entry.ToolCalls, oaRequestToolCall{
				ID:       call.ID,
				Type:     "function",
				Function: oaRequestFunction{Name: call.Name, Arguments: call.Arguments},
			})
		}
		messages = append(messages, entry)
	}

	tools := make([]oaRequestTool, 0, len(req.Tools))
	for _, tool := range req.Tools {
		tools = append(tools, oaRequestTool{
			Type: "function",
			Function: oaRequestToolFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Parameters,
			},
		})
	}

	body, err := encodeJSON(oaRequest{
		Model:    req.Model,
		Messages: messages,
		Tools:    tools,
		Stream:   stream,
	})
	if err != nil {
		return "", nil, nil, ProviderError(err.Error())
	}
	headers := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + o.apiKey,
	}
	return o.baseURL + "/chat/completions", headers, body, nil
}

func messageContent(message Message) any {
	if len(message.Images) == 0 {
		if message.Content == "" && message.Role != "assistant" && message.Role != "tool" {
			return nil
		}
		return message.Content
	}
	parts := make([]any, 0, len(message.Images)+1)
	if message.Content != "" {
		parts = append(parts, map[string]any{"type": "text", "text": message.Content})
	}
	for _, image := range message.Images {
		parts = append(parts, map[string]any{
			"type":      "image_url",
			"image_url": map[string]any{"url": image.URL},
		})
	}
	return parts
}

func (o *OpenAI) runRequest(ctx context.Context, url string, headers map[string]string, body []byte, stream bool, emit func(StreamEvent)) (*Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, ProviderError(err.Error())
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}

	response, err := o.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ProviderError("interrupted")
		}
		return nil, TransportError(err.Error())
	}
	defer response.Body.Close()

	acc := &streamAcc{}
	stalled := false
	timer := time.AfterFunc(idleReadTimeout, func() {
		stalled = true
		cancel()
	})
	reader := &idleReader{source: response.Body, timer: timer}
	_, copyErr := io.Copy(writerFunc(func(p []byte) (int, error) {
		acc.feed(p, emit, stream)
		return len(p), nil
	}), reader)
	timer.Stop()
	if copyErr != nil {
		if stalled {
			return nil, TransportError("operation timed out: stalled below 1 byte/s")
		}
		if ctx.Err() != nil {
			return nil, ProviderError("interrupted")
		}
		return nil, TransportError(copyErr.Error())
	}

	status := response.StatusCode
	if stream && status == http.StatusOK {
		if acc.streamError != nil {
			return nil, ProviderError("openai: " + *acc.streamError)
		}
		if acc.events == 0 {
			snippet := acc.raw
			if len(snippet) > 200 {
				snippet = snippet[:200]
			}
			return nil, ProviderError(fmt.Sprintf("openai: invalid response body: %s", snippet))
		}
		result := acc.response()
		acc.finish(emit)
		return result, nil
	}

	if status != http.StatusOK {
		message := ""
		var payload oaError
		if json.Unmarshal(acc.raw, &payload) == nil {
			message = payload.Error.Message
		}
		if message != "" {
			return nil, HTTPError(status, fmt.Sprintf("openai: %d: %s", status, message))
		}
		return nil, HTTPError(status, fmt.Sprintf("openai: unexpected status %d", status))
	}

	var parsed oaResponse
	if err := json.Unmarshal(acc.raw, &parsed); err != nil {
		return nil, ProviderError(err.Error())
	}
	if len(parsed.Choices) == 0 {
		return nil, ProviderError("openai: no choices in response")
	}
	choice := parsed.Choices[0]
	message := Message{Role: "assistant", Content: choice.Message.Content}
	message.Reasoning = choice.Message.ReasoningContent
	for _, call := range choice.Message.ToolCalls {
		message.ToolCalls = append(message.ToolCalls, ToolCall{
			ID:        call.ID,
			Name:      call.Function.Name,
			Arguments: call.Function.Arguments,
		})
	}
	stopReason := ""
	if choice.FinishReason != nil {
		stopReason = *choice.FinishReason
	}
	return &Response{
		Message: message,
		Usage: Usage{
			Input:       parsed.Usage.PromptTokens,
			CachedInput: parsed.Usage.PromptTokensDetails.CachedTokens,
			Output:      parsed.Usage.CompletionTokens,
		},
		StopReason: stopReason,
	}, nil
}

type idleReader struct {
	source io.ReadCloser
	timer  *time.Timer
}

func (r *idleReader) Read(p []byte) (int, error) {
	r.timer.Reset(idleReadTimeout)
	return r.source.Read(p)
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) {
	return f(p)
}

type streamAcc struct {
	raw          []byte
	buf          []byte
	content      string
	reasoning    string
	calls        []oaToolCallDelta
	usage        oaUsage
	outTokens    int
	finishReason *string
	streamError  *string
	events       int
}

func (a *streamAcc) feed(data []byte, emit func(StreamEvent), stream bool) {
	rawLimit := int(^uint(0) >> 1)
	if stream {
		rawLimit = streamRawLimit
	}
	remaining := rawLimit - len(a.raw)
	if remaining < 0 {
		remaining = 0
	}
	a.raw = append(a.raw, data[:minimum(len(data), remaining)]...)

	if bytes.IndexByte(data, '\r') >= 0 || (len(a.buf) > 0 && a.buf[len(a.buf)-1] == '\r') {
		prevCr := len(a.buf) > 0 && a.buf[len(a.buf)-1] == '\r'
		for _, b := range data {
			if b == '\n' && prevCr {
				a.buf = a.buf[:len(a.buf)-1]
			}
			a.buf = append(a.buf, b)
			prevCr = b == '\r'
		}
	} else {
		a.buf = append(a.buf, data...)
	}

	end := 0
	for {
		sep := bytes.Index(a.buf[end:], []byte("\n\n"))
		if sep < 0 {
			break
		}
		end += sep + 2
	}
	if end == 0 {
		return
	}
	remainingBuf := make([]byte, len(a.buf)-end)
	copy(remainingBuf, a.buf[end:])
	complete := a.buf[:end]
	a.buf = remainingBuf
	start := 0
	for {
		sep := bytes.Index(complete[start:], []byte("\n\n"))
		if sep < 0 {
			break
		}
		eventEnd := start + sep + 2
		a.handleEvent(complete[start:eventEnd], emit)
		start = eventEnd
	}
}

func (a *streamAcc) handleEvent(event []byte, emit func(StreamEvent)) {
	payload := strings.Builder{}
	for _, line := range bytes.Split(event, []byte("\n")) {
		text := string(line)
		data, found := strings.CutPrefix(text, "data:")
		if !found {
			continue
		}
		if payload.Len() > 0 {
			payload.WriteByte('\n')
		}
		payload.WriteString(strings.TrimSpace(data))
	}
	if payload.Len() == 0 {
		return
	}
	a.events++
	raw := payload.String()
	if raw == "[DONE]" {
		return
	}
	var chunk oaStreamChunk
	if err := json.Unmarshal([]byte(raw), &chunk); err != nil {
		return
	}
	if chunk.Error != nil {
		a.streamError = &chunk.Error.Message
		return
	}
	if emit != nil && (chunk.Usage.PromptTokens != 0 || chunk.Usage.CompletionTokens != 0) {
		a.usage = chunk.Usage
		emit(StreamEvent{
			Kind:        StreamTokens,
			Input:       a.usage.PromptTokens,
			Output:      a.usage.CompletionTokens,
			CachedInput: a.usage.PromptTokensDetails.CachedTokens,
		})
	}
	if len(chunk.Choices) == 0 {
		return
	}
	choice := chunk.Choices[0]
	if choice.FinishReason != nil {
		a.finishReason = choice.FinishReason
	}
	a.reasoning += choice.Delta.ReasoningContent
	if choice.Delta.Content != nil && *choice.Delta.Content != "" {
		a.content += *choice.Delta.Content
		a.outTokens++
		if emit != nil {
			emit(StreamEvent{Kind: StreamContent, Content: *choice.Delta.Content})
			emit(StreamEvent{
				Kind:        StreamTokens,
				Input:       a.usage.PromptTokens,
				Output:      a.outTokens,
				CachedInput: a.usage.PromptTokensDetails.CachedTokens,
			})
		}
	}
	for _, call := range choice.Delta.ToolCalls {
		if call.Index >= maxStreamToolCalls {
			continue
		}
		for len(a.calls) <= call.Index {
			a.calls = append(a.calls, oaToolCallDelta{})
		}
		entry := &a.calls[call.Index]
		if call.ID != nil {
			entry.ID = call.ID
		}
		if call.Function != nil {
			if entry.Function == nil {
				entry.Function = &oaFunctionDelta{}
			}
			if call.Function.Name != nil {
				entry.Function.Name = call.Function.Name
			}
			if call.Function.Arguments != nil {
				if entry.Function.Arguments == nil {
					empty := ""
					entry.Function.Arguments = &empty
				}
				*entry.Function.Arguments += *call.Function.Arguments
			}
		}
	}
}

func (a *streamAcc) toolCalls() []ToolCall {
	calls := make([]ToolCall, 0, len(a.calls))
	for _, call := range a.calls {
		entry := ToolCall{}
		if call.ID != nil {
			entry.ID = *call.ID
		}
		if call.Function != nil {
			if call.Function.Name != nil {
				entry.Name = *call.Function.Name
			}
			if call.Function.Arguments != nil {
				entry.Arguments = *call.Function.Arguments
			}
		}
		calls = append(calls, entry)
	}
	return calls
}

func (a *streamAcc) finish(emit func(StreamEvent)) {
	if emit == nil {
		return
	}
	for _, call := range a.toolCalls() {
		emit(StreamEvent{Kind: StreamToolCall, ToolCall: call})
	}
	emit(StreamEvent{Kind: StreamDone})
}

func (a *streamAcc) response() *Response {
	stopReason := ""
	if a.finishReason != nil {
		stopReason = *a.finishReason
	}
	return &Response{
		Message: Message{
			Role:      "assistant",
			Content:   a.content,
			ToolCalls: a.toolCalls(),
			Reasoning: a.reasoning,
		},
		Usage: Usage{
			Input:       a.usage.PromptTokens,
			Output:      a.usage.CompletionTokens,
			CachedInput: a.usage.PromptTokensDetails.CachedTokens,
		},
		StopReason: stopReason,
	}
}

func minimum(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type oaRequest struct {
	Model    string             `json:"model"`
	Messages []oaRequestMessage `json:"messages"`
	Tools    []oaRequestTool    `json:"tools,omitempty"`
	Stream   bool               `json:"stream,omitempty"`
}

type oaRequestMessage struct {
	Role             string              `json:"role"`
	Content          any                 `json:"content,omitempty"`
	ReasoningContent string              `json:"reasoning_content,omitempty"`
	ToolCalls        []oaRequestToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string              `json:"tool_call_id,omitempty"`
}

type oaRequestToolCall struct {
	ID       string            `json:"id"`
	Type     string            `json:"type"`
	Function oaRequestFunction `json:"function"`
}

type oaRequestFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type oaRequestTool struct {
	Type     string                `json:"type"`
	Function oaRequestToolFunction `json:"function"`
}

type oaRequestToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

type oaMessage struct {
	Content          string       `json:"content"`
	ReasoningContent string       `json:"reasoning_content"`
	ToolCalls        []oaToolCall `json:"tool_calls"`
}

type oaToolCall struct {
	ID       string     `json:"id"`
	Function oaFunction `json:"function"`
}

type oaFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type oaResponse struct {
	Choices []oaChoice `json:"choices"`
	Usage   oaUsage    `json:"usage"`
}

type oaChoice struct {
	Message      oaMessage `json:"message"`
	FinishReason *string   `json:"finish_reason"`
}

type oaError struct {
	Error oaErrorPayload `json:"error"`
}

type oaErrorPayload struct {
	Message string `json:"message"`
}

type oaUsage struct {
	PromptTokens        int                   `json:"prompt_tokens"`
	CompletionTokens    int                   `json:"completion_tokens"`
	PromptTokensDetails oaPromptTokensDetails `json:"prompt_tokens_details"`
}

type oaPromptTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

type oaToolCallDelta struct {
	Index    int              `json:"index"`
	ID       *string          `json:"id"`
	Function *oaFunctionDelta `json:"function"`
}

type oaFunctionDelta struct {
	Name      *string `json:"name"`
	Arguments *string `json:"arguments"`
}

type oaDeltaMessage struct {
	Content          *string           `json:"content"`
	ReasoningContent string            `json:"reasoning_content"`
	ToolCalls        []oaToolCallDelta `json:"tool_calls"`
}

type oaStreamChoice struct {
	Delta        oaDeltaMessage `json:"delta"`
	FinishReason *string        `json:"finish_reason"`
}

type oaStreamChunk struct {
	Choices []oaStreamChoice `json:"choices"`
	Usage   oaUsage          `json:"usage"`
	Error   *oaStreamError   `json:"error"`
}

type oaStreamError struct {
	Message string `json:"message"`
}

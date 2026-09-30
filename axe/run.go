package axe

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

const maxRetries = 2

type Sink interface {
	AssistantDelta(text string)
	AssistantDone()
	ToolStart(call ToolCall)
	ToolDelta(call ToolCall, text string)
	ToolResult(call ToolCall, output ToolOutput, elapsed time.Duration)
	Tokens(input, output, cachedInput int)
	ShouldCompact(input, output int) bool
	Assistant(turn int, message Message, usage Usage)
	Tool(turn int, message Message)
	PendingUserInput() string
}

type SinkBase struct{}

func (SinkBase) AssistantDelta(string)                          {}
func (SinkBase) AssistantDone()                                 {}
func (SinkBase) ToolStart(ToolCall)                             {}
func (SinkBase) ToolDelta(ToolCall, string)                     {}
func (SinkBase) ToolResult(ToolCall, ToolOutput, time.Duration) {}
func (SinkBase) Tokens(int, int, int)                           {}
func (SinkBase) ShouldCompact(int, int) bool                    { return false }
func (SinkBase) Assistant(int, Message, Usage)                  {}
func (SinkBase) Tool(int, Message)                              {}
func (SinkBase) PendingUserInput() string                       { return "" }

type RunOptions struct {
	Model    string
	System   string
	Tools    []Tool
	MaxTurns int
}

type OutcomeKind int

const (
	OutcomeDone OutcomeKind = iota
	OutcomeMaxTurns
	OutcomeCancelled
	OutcomeCompact
	OutcomeFailed
)

type Outcome struct {
	Kind    OutcomeKind
	Failure string
}

type RunEnd struct {
	Messages []Message
	Usage    Usage
	Context  Usage
	Outcome  Outcome
}

func RunStream(ctx context.Context, provider Provider, opts *RunOptions, messages []Message, sink Sink) RunEnd {
	if ctx == nil {
		ctx = context.Background()
	}
	started := time.Now()
	history := make([]Message, 0, len(messages))
	for _, message := range messages {
		history = append(history, RedactMessage(message))
	}
	usage := Usage{}
	contextUsage := Usage{}
	trace(fmt.Sprintf("run start messages=%d system_bytes=%d tools=%d", len(messages), len(opts.System), len(opts.Tools)))

	for turn := 0; turn < opts.MaxTurns; turn++ {
		trace(fmt.Sprintf("turn=%d start messages=%d", turn+1, len(history)))
		if cancelled(ctx) {
			return RunEnd{Messages: history, Usage: usage, Context: contextUsage, Outcome: Outcome{Kind: OutcomeCancelled}}
		}
		if text := sink.PendingUserInput(); text != "" {
			history = append(history, userMessage(text))
		}
		response, calls, err := streamOnce(ctx, provider, opts, history, sink)
		if err != nil {
			if cancelled(ctx) {
				return RunEnd{Messages: history, Usage: usage, Context: contextUsage, Outcome: Outcome{Kind: OutcomeCancelled}}
			}
			return RunEnd{Messages: history, Usage: usage, Context: contextUsage, Outcome: Outcome{Kind: OutcomeFailed, Failure: err.Error()}}
		}
		trace(fmt.Sprintf(
			"turn=%d model_done elapsed_ms=%d input_tokens=%d cached_input_tokens=%d output_tokens=%d tool_calls=%d",
			turn+1, started.UnixMilli(), response.Usage.Input, response.Usage.CachedInput, response.Usage.Output, len(calls),
		))
		usage = usage.Add(response.Usage)
		contextUsage = response.Usage
		history = append(history, response.Message)
		sink.Assistant(turn, history[len(history)-1], response.Usage)
		sink.AssistantDone()
		if len(calls) == 0 {
			if text := sink.PendingUserInput(); text != "" {
				history = append(history, userMessage(text))
				continue
			}
			trace(fmt.Sprintf(
				"run done elapsed_ms=%d turns=%d input_tokens=%d cached_input_tokens=%d output_tokens=%d",
				started.UnixMilli(), turn+1, usage.Input, usage.CachedInput, usage.Output,
			))
			return RunEnd{Messages: history, Usage: usage, Context: contextUsage, Outcome: Outcome{Kind: OutcomeDone}}
		}
		truncated := response.StopReason == "length"
		if !runToolBatch(opts.Tools, calls, truncated, turn, ctx, sink, &history) {
			return RunEnd{Messages: history, Usage: usage, Context: contextUsage, Outcome: Outcome{Kind: OutcomeCancelled}}
		}
		if sink.ShouldCompact(response.Usage.Input, response.Usage.Output) {
			if text := sink.PendingUserInput(); text != "" {
				history = append(history, userMessage(text))
			}
			return RunEnd{Messages: history, Usage: usage, Context: contextUsage, Outcome: Outcome{Kind: OutcomeCompact}}
		}
	}
	return RunEnd{Messages: history, Usage: usage, Context: contextUsage, Outcome: Outcome{Kind: OutcomeMaxTurns}}
}

func userMessage(text string) Message {
	return Message{Role: "user", Content: Redact(text)}
}

func runToolBatch(tools []Tool, calls []ToolCall, truncated bool, turn int, ctx context.Context, sink Sink, history *[]Message) bool {
	anySequential := false
	for _, call := range calls {
		for _, tool := range tools {
			if tool.Name == call.Name && tool.Sequential {
				anySequential = true
			}
		}
	}
	if truncated || anySequential || len(calls) <= 1 {
		interrupted := false
		for _, call := range calls {
			if cancelled(ctx) {
				interrupted = true
			}
			sink.ToolStart(call)
			var output ToolOutput
			var elapsed time.Duration
			switch {
			case interrupted:
				output = TextOutput("error: tool call not executed: the run was interrupted.")
			case truncated:
				output = TextOutput("error: tool call not executed: the response hit the output token limit, so its arguments may be truncated. Re-issue the tool call with complete arguments.")
			default:
				output, elapsed = execTool(tools, call, sink)
			}
			sink.ToolResult(call, output, elapsed)
			pushToolResult(history, call, output)
			sink.Tool(turn, (*history)[len(*history)-1])
		}
		return !cancelled(ctx)
	}
	return runParallel(tools, calls, turn, ctx, sink, history)
}

func pushToolResult(history *[]Message, call ToolCall, output ToolOutput) {
	*history = append(*history, Message{
		Role:       "tool",
		Content:    Redact(output.Text),
		ToolCallID: call.ID,
		Images:     output.Images,
	})
}

type parallelMsg struct {
	index   int
	delta   string
	output  ToolOutput
	elapsed time.Duration
	done    bool
}

func runParallel(tools []Tool, calls []ToolCall, turn int, ctx context.Context, sink Sink, history *[]Message) bool {
	for _, call := range calls {
		sink.ToolStart(call)
	}
	messages := make(chan parallelMsg, 16)
	var wait sync.WaitGroup
	for index, call := range calls {
		wait.Add(1)
		go func(index int, call ToolCall) {
			defer wait.Done()
			if cancelled(ctx) {
				messages <- parallelMsg{index: index, done: true, output: TextOutput("error: tool call not executed: the run was interrupted.")}
				return
			}
			output, elapsed := runTool(tools, call, func(text string) {
				messages <- parallelMsg{index: index, delta: text}
			})
			messages <- parallelMsg{index: index, done: true, output: output, elapsed: elapsed}
		}(index, call)
	}
	go func() {
		wait.Wait()
		close(messages)
	}()

	outputs := make([]parallelMsg, len(calls))
	for message := range messages {
		if message.done {
			outputs[message.index] = message
			continue
		}
		sink.ToolDelta(calls[message.index], message.delta)
	}
	for index, call := range calls {
		sink.ToolResult(call, outputs[index].output, outputs[index].elapsed)
		pushToolResult(history, call, outputs[index].output)
		sink.Tool(turn, (*history)[len(*history)-1])
	}
	return !cancelled(ctx)
}

func runTool(tools []Tool, call ToolCall, progress Progress) (ToolOutput, time.Duration) {
	started := time.Now()
	for _, tool := range tools {
		if tool.Name == call.Name {
			output := tool.Run(call.Arguments, progress)
			trace(fmt.Sprintf(
				"tool name=%s elapsed_ms=%d argument_bytes=%d output_bytes=%d",
				call.Name, time.Since(started).Milliseconds(), len(call.Arguments), len(output.Text),
			))
			return output, time.Since(started)
		}
	}
	trace(fmt.Sprintf("tool name=%s elapsed_ms=%d unknown=true", call.Name, time.Since(started).Milliseconds()))
	return TextOutput(fmt.Sprintf("error: unknown tool: %s", call.Name)), time.Since(started)
}

func execTool(tools []Tool, call ToolCall, sink Sink) (ToolOutput, time.Duration) {
	return runTool(tools, call, func(text string) { sink.ToolDelta(call, text) })
}

func streamOnce(ctx context.Context, provider Provider, opts *RunOptions, history []Message, sink Sink) (*Response, []ToolCall, error) {
	request := &Request{
		Context:  ctx,
		Model:    opts.Model,
		System:   opts.System,
		Messages: history,
		Tools:    opts.Tools,
	}
	attempt := 0
	for {
		started := time.Now()
		handle := provider.Stream(request)
		var calls []ToolCall
		forwarded := 0
		firstEvent := false

	drain:
		for event := range handle.Events {
			switch event.Kind {
			case StreamContent:
				if !firstEvent {
					trace(fmt.Sprintf("model first_event_ms=%d", time.Since(started).Milliseconds()))
					firstEvent = true
				}
				forwarded++
				sink.AssistantDelta(event.Content)
			case StreamToolCall:
				if !firstEvent {
					trace(fmt.Sprintf("model first_event_ms=%d", time.Since(started).Milliseconds()))
					firstEvent = true
				}
				forwarded++
				calls = append(calls, event.ToolCall)
			case StreamTokens:
				sink.Tokens(event.Input, event.Output, event.CachedInput)
			case StreamDone:
				break drain
			}
		}
		response, err := handle.Join()
		if err == nil {
			trace(fmt.Sprintf("model request_done_ms=%d attempt=%d", time.Since(started).Milliseconds(), attempt+1))
			return response, calls, nil
		}
		if attempt >= maxRetries || forwarded > 0 || !retryableError(err) {
			return nil, nil, err
		}
		attempt++
		delay := backoff(attempt)
		trace(fmt.Sprintf("model retry=%d backoff_ms=%d error=%s", attempt, delay, err))
		if sleepErr := sleepWithCancel(ctx, delay); sleepErr != nil {
			return nil, nil, sleepErr
		}
	}
}

func retryableError(err error) bool {
	var failure *Error
	if !errors.As(err, &failure) {
		return false
	}
	if failure.Kind == ErrTransport {
		return true
	}
	if failure.Kind != ErrHTTP {
		return false
	}
	return failure.Status == 408 || failure.Status == 409 || failure.Status == 429 || failure.Status >= 500
}

func backoff(attempt int) int {
	shift := attempt - 1
	if shift > 4 {
		shift = 4
	}
	base := 500 << shift
	if base > 8000 {
		base = 8000
	}
	now := uint64(time.Now().UnixNano())
	return base - int(now%(uint64(base/4)+1))
}

func cancelled(ctx context.Context) bool {
	if ctx != nil && ctx.Err() != nil {
		KillChildren()
		return true
	}
	return false
}

func sleepWithCancel(ctx context.Context, milliseconds int) error {
	deadline := time.Now().Add(time.Duration(milliseconds) * time.Millisecond)
	for time.Now().Before(deadline) {
		if cancelled(ctx) {
			return ProviderError("interrupted")
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil
}

func trace(message string) {
	if os.Getenv("AXE_TRACE") != "" {
		fmt.Fprintln(os.Stderr, "trace: "+message)
	}
}

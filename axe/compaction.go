package axe

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const retainTokens = 20_000

func LatestContextTokens(entries []Entry) (int, bool) {
	for index := len(entries) - 1; index >= 0; index-- {
		entry := entries[index]
		switch entry.Type {
		case EntryUsage:
			if entry.ContextInput > 0 {
				return entry.ContextInput + entry.ContextOutput, true
			}
		case EntryCompaction:
			return 0, false
		}
	}
	return 0, false
}

func splitRetained(entries []Entry, options ContextOptions) ([]Message, []Message) {
	msgs := ContextMessagesWith(entries, options)
	currentTokens, hasTokens := LatestContextTokens(entries)
	if !hasTokens {
		return splitLastTurn(msgs)
	}
	messageCount := len(msgs)
	retainedStart := messageCount
	activeStart := 0
	for index := len(entries) - 1; index >= 0; index-- {
		if entries[index].Type == EntryCompaction {
			activeStart = index + 1
			break
		}
	}

scan:
	for index := len(entries) - 1; index >= activeStart; index-- {
		entry := entries[index]
		switch entry.Type {
		case EntryMessage:
			if messageCount > 0 {
				messageCount--
			}
		case EntryUsage:
			if entry.ContextInput > 0 {
				boundary := entry.ContextInput + entry.ContextOutput
				if saturatingSub(currentTokens, boundary) > retainTokens {
					break scan
				}
				retainedStart = messageCount
			}
		}
	}
	if retainedStart == len(msgs) {
		return splitLastTurn(msgs)
	}
	return msgs[retainedStart:], msgs[:retainedStart]
}

func splitLastTurn(msgs []Message) ([]Message, []Message) {
	start := len(msgs)
	for index := len(msgs) - 1; index >= 0; index-- {
		if msgs[index].Role == "user" {
			start = index
			break
		}
	}
	return msgs[start:], msgs[:start]
}

func saturatingSub(a, b int) int {
	if b > a {
		return 0
	}
	return a - b
}

func serializeConversation(msgs []Message) string {
	tools := map[string]string{}
	var out strings.Builder
	appendBlock := func(block string) {
		if out.Len() > 0 {
			out.WriteString("\n\n")
		}
		out.WriteString(block)
	}
	for _, message := range msgs {
		switch message.Role {
		case "user":
			appendBlock("[User]: " + message.Content)
		case "assistant":
			if message.Content != "" {
				appendBlock("[Assistant]: " + message.Content)
			}
			for _, call := range message.ToolCalls {
				tools[call.ID] = call.Name
				appendBlock(fmt.Sprintf("[Tool call %s]: %s(%s)", call.ID, call.Name, call.Arguments))
			}
		case "tool":
			limit := 4000
			if tools[message.ToolCallID] == "read" {
				limit = 1200
			}
			appendBlock(fmt.Sprintf(
				"[Tool result %s; full result remains in session]: %s",
				message.ToolCallID, compactObservation(message.Content, limit),
			))
		}
	}
	return out.String()
}

func compactObservation(content string, limit int) string {
	if len(content) <= limit {
		return content
	}
	half := limit / 2
	headEnd := half
	for headEnd > 0 && !utf8.RuneStart(content[headEnd]) {
		headEnd--
	}
	tailStart := len(content) - half
	for tailStart < len(content) && !utf8.RuneStart(content[tailStart]) {
		tailStart++
	}
	return fmt.Sprintf(
		"%s\n… [%d bytes masked] …\n%s",
		content[:headEnd], saturatingSub(tailStart, headEnd), content[tailStart:],
	)
}

const summarySystem = "You are a context summarization assistant. Read the conversation and produce a structured summary so another LLM can continue the work. Do NOT continue the conversation. Do NOT respond to questions in it. ONLY output the summary."

const summaryPrompt = `Create a factual context checkpoint that another coding agent will use to continue the work.

Use these exact headings:

## Goal
## User Requirements
## Progress
### Done
### In Progress
### Blocked
## Key Decisions
## Files
## Commands and Results
## Open Questions
## Next Steps
## Critical Context

Use concise bullets. Record only facts supported by the conversation. Distinguish completed work from proposed work. Preserve exact file paths, symbol names, commands, exit status, error messages, values, and user requirements. Keep still-active facts from an earlier checkpoint. Remove superseded facts. Do not copy large tool outputs or source files; retain only details needed to continue.`

var summaryHeadings = []string{
	"## Goal",
	"## User Requirements",
	"## Progress",
	"### Done",
	"### In Progress",
	"### Blocked",
	"## Key Decisions",
	"## Files",
	"## Commands and Results",
	"## Open Questions",
	"## Next Steps",
	"## Critical Context",
}

func validSummary(summary string) bool {
	summary = strings.TrimSpace(summary)
	if len(summary) < 100 {
		return false
	}
	for _, heading := range summaryHeadings {
		if !strings.Contains(summary, heading) {
			return false
		}
	}
	return true
}

func fallbackSummary(msgs []Message, candidate string) string {
	facts := []string{}
	size := 0
	for index := len(msgs) - 1; index >= 0; index-- {
		message := msgs[index]
		if message.Content == "" || (message.Role != "user" && message.Role != "assistant") {
			continue
		}
		available := saturatingSub(6000, size)
		if available < 200 {
			break
		}
		content := compactObservation(message.Content, minimum(available, 1200))
		size += len(content)
		facts = append(facts, fmt.Sprintf("- %s: %s", message.Role, content))
	}
	reverse(facts)
	trimmed := strings.TrimSpace(candidate)
	suffix := ""
	if trimmed != "" {
		suffix = "\n- Model checkpoint: " + compactObservation(trimmed, 1200)
	}
	return "## Goal\n- Continue the original task\n" +
		"## User Requirements\n- See original task and retained messages\n" +
		"## Progress\n### Done\n- See critical context\n" +
		"### In Progress\n- Continue from the latest retained state\n" +
		"### Blocked\n- Unknown\n" +
		"## Key Decisions\n- See critical context\n" +
		"## Files\n- See deterministic workspace state\n" +
		"## Commands and Results\n- See critical context\n" +
		"## Open Questions\n- Re-evaluate from retained state\n" +
		"## Next Steps\n- Continue from the latest retained state\n" +
		"## Critical Context\n" + strings.Join(facts, "\n") + suffix
}

func reverse(values []string) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func requestSummary(provider Provider, model, conversation, correction string) (string, error) {
	prompt := "<conversation>\n" + conversation + "\n</conversation>\n\n" + summaryPrompt + "\n\n" + correction
	request := &Request{
		Model:  model,
		System: summarySystem,
		Messages: []Message{{
			Role:    "user",
			Content: prompt,
		}},
	}
	response, err := provider.Complete(request)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(response.Message.Content), nil
}

func Compact(provider Provider, model string, entries []Entry) (string, int, []Message, error) {
	return CompactWith(provider, model, entries, DefaultContextOptions())
}

func CompactWith(provider Provider, model string, entries []Entry, options ContextOptions) (string, int, []Message, error) {
	tokensBefore, _ := LatestContextTokens(entries)
	retained, toSummarize := splitRetained(entries, options)
	retained = DropIncompleteToolCalls(retained)
	if len(toSummarize) == 0 {
		return "", 0, nil, errors.New("nothing to summarize")
	}
	conversation := serializeConversation(toSummarize)
	first, _ := requestSummary(provider, model, conversation, "")
	summary := ""
	if validSummary(first) {
		summary = first
	} else {
		second, _ := requestSummary(provider, model, conversation,
			"Your previous response was invalid. Return all required headings and substantive factual content.")
		if validSummary(second) {
			summary = second
		} else {
			candidate := second
			if candidate == "" {
				candidate = first
			}
			summary = fallbackSummary(toSummarize, candidate)
		}
	}
	return summary, tokensBefore, retained, nil
}

var overflowPatterns = []string{
	"prompt is too long",
	"exceeds the context window",
	"maximum context length",
	"input token count",
	"context_length_exceeded",
	"prompt too long",
	"exceeds the model's maximum",
}

var nonOverflowPatterns = []string{"throttling", "rate limit", "service unavailable"}

func IsOverflowError(err string) bool {
	message := strings.ToLower(err)
	for _, pattern := range nonOverflowPatterns {
		if strings.Contains(message, pattern) {
			return false
		}
	}
	for _, pattern := range overflowPatterns {
		if strings.Contains(message, pattern) {
			return true
		}
	}
	return false
}

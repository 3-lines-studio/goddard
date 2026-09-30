package axe

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type ToolCall struct {
	ID        string `json:"ID"`
	Name      string `json:"Name"`
	Arguments string `json:"Arguments"`
}

type Image struct {
	Path string `json:"Path,omitempty"`
	URL  string `json:"URL"`
}

type Message struct {
	Role       string     `json:"Role"`
	Content    string     `json:"Content"`
	ToolCalls  []ToolCall `json:"ToolCalls,omitempty"`
	ToolCallID string     `json:"ToolCallID,omitempty"`
	Reasoning  string     `json:"Reasoning,omitempty"`
	Images     []Image    `json:"Images,omitempty"`
}

type Usage struct {
	Input       int
	Output      int
	CachedInput int
}

func (u Usage) Add(other Usage) Usage {
	return Usage{
		Input:       u.Input + other.Input,
		Output:      u.Output + other.Output,
		CachedInput: u.CachedInput + other.CachedInput,
	}
}

type StreamEventKind int

const (
	StreamContent StreamEventKind = iota
	StreamToolCall
	StreamTokens
	StreamDone
)

type StreamEvent struct {
	Kind        StreamEventKind
	Content     string
	ToolCall    ToolCall
	Input       int
	Output      int
	CachedInput int
}

type ErrorKind int

const (
	ErrTransport ErrorKind = iota
	ErrProvider
	ErrHTTP
)

type Error struct {
	Kind    ErrorKind
	Status  int
	Message string
}

func (e *Error) Error() string {
	return e.Message
}

func TransportError(message string) *Error {
	return &Error{Kind: ErrTransport, Message: message}
}

func ProviderError(message string) *Error {
	return &Error{Kind: ErrProvider, Message: message}
}

func HTTPError(status int, message string) *Error {
	return &Error{Kind: ErrHTTP, Status: status, Message: message}
}

type ToolOutput struct {
	Text   string
	Images []Image
}

func TextOutput(text string) ToolOutput {
	return ToolOutput{Text: text}
}

type Progress func(text string)

func (p Progress) Report(text string) {
	if p != nil {
		p(text)
	}
}

type Tool struct {
	Name        string
	Description string
	Parameters  any
	Snippet     string
	Sequential  bool
	Run         func(raw string, progress Progress) ToolOutput
}

type Output interface {
	ToolOutput | string
}

func NewTool[T any, R Output](name, description, schema string, run func(T) R) Tool {
	return NewToolWithProgress(name, description, schema, func(args T, _ Progress) R {
		return run(args)
	})
}

func NewToolWithProgress[T any, R Output](name, description, schema string, run func(T, Progress) R) Tool {
	var parameters any
	if err := json.Unmarshal([]byte(schema), &parameters); err != nil {
		parameters = nil
	}
	return Tool{
		Name:        name,
		Description: description,
		Parameters:  parameters,
		Run: func(raw string, progress Progress) ToolOutput {
			if strings.TrimSpace(raw) == "" {
				raw = "{}"
			}
			args := parseArgs(raw)
			if args != nil {
				coerceArgs(&args, parameters)
			}
			var typed T
			if _, custom := any(&typed).(json.Unmarshaler); !custom {
				if reason := checkArgs(parameters, args); reason != "" {
					return TextOutput(invalidArgs(name, reason, raw))
				}
			}
			coerced, err := json.Marshal(args)
			if err != nil {
				coerced = []byte(raw)
			}
			if err := json.Unmarshal(coerced, &typed); err != nil {
				return TextOutput(invalidArgs(name, err.Error(), raw))
			}
			return asOutput(run(typed, progress))
		},
	}
}

func invalidArgs(name, reason, raw string) string {
	return fmt.Sprintf("error: invalid arguments for %s: %s\nReceived: %s", name, reason, raw)
}

func asOutput[R Output](value R) ToolOutput {
	switch v := any(value).(type) {
	case ToolOutput:
		return v
	case string:
		return ToolOutput{Text: v}
	default:
		return ToolOutput{}
	}
}

func parseArgs(raw string) any {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var args any
	if err := decoder.Decode(&args); err != nil {
		return nil
	}
	return args
}

func coerceArgs(args *any, schema any) {
	schemaObject, ok := schema.(map[string]any)
	if !ok {
		return
	}
	if object, isObject := (*args).(map[string]any); isObject {
		if properties, hasProperties := schemaObject["properties"].(map[string]any); hasProperties {
			required := requiredSet(schemaObject)
			for key, property := range properties {
				value, present := object[key]
				if !present {
					continue
				}
				if value == nil && !required[key] && !acceptsNull(property) {
					delete(object, key)
					continue
				}
				coerceValue(&value, property)
				coerceArgs(&value, property)
				object[key] = value
			}
		}
	}
	if items, hasItems := schemaObject["items"]; hasItems {
		if array, isArray := (*args).([]any); isArray {
			for index := range array {
				coerceValue(&array[index], items)
				coerceArgs(&array[index], items)
			}
		}
	}
}

func requiredSet(schemaObject map[string]any) map[string]bool {
	required := map[string]bool{}
	if list, ok := schemaObject["required"].([]any); ok {
		for _, item := range list {
			if key, isString := item.(string); isString {
				required[key] = true
			}
		}
	}
	return required
}

func checkArgs(schema any, args any) string {
	schemaObject, ok := schema.(map[string]any)
	if !ok {
		return ""
	}
	required, hasRequired := schemaObject["required"].([]any)
	if !hasRequired || len(required) == 0 {
		return ""
	}
	object, isObject := args.(map[string]any)
	if !isObject {
		return "invalid type: expected an object"
	}
	for _, item := range required {
		key, isString := item.(string)
		if !isString {
			continue
		}
		if _, present := object[key]; !present {
			return fmt.Sprintf("missing field `%s`", key)
		}
	}
	return ""
}

func acceptsNull(schema any) bool {
	schemaObject, ok := schema.(map[string]any)
	if !ok {
		return false
	}
	switch declared := schemaObject["type"].(type) {
	case string:
		return declared == "null"
	case []any:
		for _, item := range declared {
			if item == "null" {
				return true
			}
		}
	}
	return false
}

const (
	minInt64Float = -9223372036854775808.0
	maxInt64Float = 9223372036854775807.0
)

func coerceValue(value *any, schema any) {
	schemaObject, ok := schema.(map[string]any)
	if !ok {
		return
	}
	declared, _ := schemaObject["type"].(string)
	switch declared {
	case "number", "integer":
		coerceNumber(value, declared)
	case "boolean":
		coerceBoolean(value)
	case "string":
		coerceString(value)
	}
}

func coerceNumber(value *any, declared string) {
	whole := func(n float64) bool {
		return declared == "integer" && !math.IsNaN(n) && !math.IsInf(n, 0) && n >= minInt64Float && n <= maxInt64Float
	}
	switch v := (*value).(type) {
	case json.Number:
		n, err := strconv.ParseFloat(string(v), 64)
		if err != nil {
			return
		}
		if whole(n) {
			*value = int64(n)
		}
	case string:
		n, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return
		}
		if whole(n) {
			*value = int64(n)
		} else if declared == "number" {
			*value = n
		}
	case nil:
		*value = int64(0)
	case bool:
		if v {
			*value = int64(1)
		} else {
			*value = int64(0)
		}
	}
}

func coerceBoolean(value *any) {
	switch v := (*value).(type) {
	case string:
		switch strings.TrimSpace(v) {
		case "true":
			*value = true
		case "false":
			*value = false
		}
	case json.Number:
		n, err := strconv.ParseFloat(string(v), 64)
		if err != nil {
			return
		}
		if n == 1 {
			*value = true
		} else if n == 0 {
			*value = false
		}
	case nil:
		*value = false
	}
}

func coerceString(value *any) {
	switch v := (*value).(type) {
	case json.Number:
		*value = string(v)
	case bool:
		*value = strconv.FormatBool(v)
	case nil:
		*value = ""
	}
}

func SystemPrompt(tools []Tool) string {
	var builder strings.Builder
	builder.WriteString("Available tools:\n")
	for _, tool := range tools {
		if tool.Snippet != "" {
			fmt.Fprintf(&builder, "- %s: %s\n", tool.Name, tool.Snippet)
		}
	}
	return builder.String()
}

func UserSystemPrompt() string {
	dir := ConfigDir()
	if dir == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(dir, "axe", "SYSTEM.md"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func ConfigDir() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return dir
	}
	home := os.Getenv("HOME")
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".config")
}

func AtomicWrite(path string, data []byte) error {
	return atomicWriteWith(path, func(file *os.File) error {
		_, err := file.Write(data)
		return err
	})
}

var tmpTag atomic.Uint64

func atomicWriteWith(path string, write func(*os.File) error) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o777); err != nil {
		return err
	}
	tag := tmpTag.Add(1) - 1
	now := time.Now()
	tmp := filepath.Join(dir, tempName(os.Getpid(), now.Unix(), uint32(now.Nanosecond()), tag))
	var permissions os.FileMode
	keepPermissions := false
	if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() {
		permissions = info.Mode().Perm()
		keepPermissions = true
	}
	file, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
	if err != nil {
		return err
	}
	err = write(file)
	if err == nil && keepPermissions {
		err = file.Chmod(permissions)
	}
	if err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err == nil {
		err = syncDir(dir)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}

func syncDir(dir string) error {
	file, err := os.Open(dir)
	if err != nil {
		return err
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

func tempName(pid int, secs int64, nanos uint32, tag uint64) string {
	return fmt.Sprintf(".axe-tmp-%d-%d-%d", pid, uint64(nanos)^uint64(secs), tag)
}

type Provider interface {
	Complete(req *Request) (*Response, error)
	Stream(req *Request) *StreamHandle
}

type Request struct {
	Context  context.Context
	Model    string
	System   string
	Messages []Message
	Tools    []Tool
}

type Response struct {
	Message    Message
	Usage      Usage
	StopReason string
}

type StreamHandle struct {
	Events <-chan StreamEvent
	join   func() (*Response, error)
}

func NewStreamHandle(events <-chan StreamEvent, join func() (*Response, error)) *StreamHandle {
	return &StreamHandle{Events: events, join: join}
}

func (h *StreamHandle) Join() (*Response, error) {
	return h.join()
}

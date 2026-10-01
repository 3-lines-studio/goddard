package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"math"
	"path/filepath"
	"strings"
	"time"

	"github.com/3-lines-studio/goddard/axe"
	"github.com/3-lines-studio/goddard/chat"
	"github.com/3-lines-studio/goddard/memo"
	"github.com/3-lines-studio/goddard/prompt"
	"github.com/3-lines-studio/goddard/schedule"
	"github.com/3-lines-studio/goddard/skill"
)

// TurnLease is how long a turn holds its conversation, in seconds. It is long
// on purpose: a tool that compiles takes a while, and a claim that expires
// mid-turn would let a second instance start the same conversation.
const TurnLease = 900

var (
	ErrNoConversation = errors.New("esa conversación no existe")
	ErrReadOnly       = errors.New("esa conversación no se escribe desde acá")
	ErrBusy           = errors.New("ya hay un turno corriendo en esa conversación")
	ErrEmpty          = errors.New("el mensaje está vacío")
)

// Say writes the message and sends the agent after it. It returns as soon as
// the turn is claimed: what the agent answers shows up in the log, and the web
// reads it from there. The uploads are the attachments of this message, by id:
// they are written to the workspace so the agent can read them, and into the
// log so the thread shows them.
func (s *Service) Say(ctx context.Context, conversationID, text string, uploads []string) error {
	conversation, ok, err := s.Chat.Conversation(ctx, conversationID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNoConversation
	}
	if conversation.Source != chat.SourceWeb {
		return ErrReadOnly
	}
	if strings.TrimSpace(text) == "" && len(uploads) == 0 {
		return ErrEmpty
	}
	taken, err := s.Chat.Claim(ctx, conversationID, TurnLease)
	if err != nil {
		return err
	}
	if !taken {
		return ErrBusy
	}
	if strings.TrimSpace(text) != "" {
		if _, err := s.write(ctx, conversationID, map[string]any{"event": "user", "text": text}); err != nil {
			_ = s.Chat.Release(ctx, conversationID)
			return err
		}
	}
	files, err := s.attach(ctx, conversation, uploads)
	if err != nil {
		_ = s.Chat.Release(ctx, conversationID)
		return err
	}
	if conversation.Title == chat.NewTitle {
		_ = s.Chat.RenameConversation(ctx, conversationID, titleOf(text+first(files)))
	}
	turn, cancel := context.WithCancel(context.Background())
	s.Stops.add(conversationID, cancel)
	go func() {
		defer s.Stops.drop(conversationID)
		_, _ = s.answer(turn, conversation, messageOf(text, files))
	}()
	return nil
}

// attached is a file of the message, already where the agent can read it.
type attached struct {
	Path string
	Mime string
}

// attach writes the files of a message into the workspace of the app, under
// the conversation they belong to, and leaves them in the log. The bytes live
// in the database, which is what any instance can serve; the workspace is
// where the agent reads them, which is where its tools look.
func (s *Service) attach(ctx context.Context, conversation chat.Conversation, uploads []string) ([]attached, error) {
	files := []attached{}
	for _, id := range uploads {
		upload, ok, err := s.Chat.Upload(ctx, id)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("ese archivo no existe")
		}
		name := filepath.Base(upload.Name)
		path := filepath.Join("files", conversation.ID, name)
		if err := s.Machine.Write(path, upload.Bytes); err != nil {
			return nil, err
		}
		if _, err := s.write(ctx, conversation.ID, map[string]any{
			"event": "file", "id": upload.ID, "name": name, "mime": upload.Mime,
		}); err != nil {
			return nil, err
		}
		files = append(files, attached{Path: path, Mime: upload.Mime})
	}
	return files, nil
}

// messageOf is what the agent reads: the message, and where the files of the
// message are.
func messageOf(text string, files []attached) string {
	if len(files) == 0 {
		return text
	}
	out := text
	if strings.TrimSpace(out) != "" {
		out += "\n\n"
	}
	out += "Archivos adjuntos:\n"
	for _, file := range files {
		out += fmt.Sprintf("- %s (%s)\n", file.Path, file.Mime)
	}
	return out
}

// first gives a message with no words something to be named after.
func first(files []attached) string {
	if len(files) == 0 {
		return ""
	}
	return filepath.Base(files[0].Path)
}

// answer runs one turn and leaves it written in the log. It returns what the
// agent said, which is what a task of the agenda is after. The request that
// asked for a turn is long gone by then, so the context is the caller's.
func (s *Service) answer(ctx context.Context, conversation chat.Conversation, text string) (string, error) {
	defer func() {
		if err := s.Chat.Release(context.WithoutCancel(ctx), conversation.ID); err != nil {
			log.Printf("goddard: no pude soltar %s: %v", conversation.ID, err)
		}
	}()
	sink := &logSink{service: s, conversationID: conversation.ID}
	store := axe.NewPgStore(s.DB, conversation.ID)
	entries, err := store.Live(ctx)
	if err != nil {
		if ctx.Err() != nil {
			sink.write(map[string]any{"event": "stopped"})
			sink.write(map[string]any{"event": "done"})
			return "", err
		}
		s.failed(ctx, conversation.ID, err)
		return "", err
	}
	messages := axe.DropIncompleteToolCalls(axe.ContextMessages(entries))
	messages = append(messages, axe.Message{Role: "user", Content: text})
	project := s.projectSlug(ctx, conversation)
	tools := s.tools(project, conversation.ID)
	options := &axe.RunOptions{
		Model:    s.Model,
		System:   s.system(ctx, tools, conversation, project),
		Tools:    tools,
		MaxTurns: math.MaxInt,
	}
	end := axe.RunStream(ctx, s.Provider, options, messages, sink)
	if len(end.Messages) > len(messages) {
		fresh := make([]axe.Entry, 0, len(end.Messages)-len(messages))
		for _, message := range end.Messages[len(messages):] {
			fresh = append(fresh, axe.MessageEntry(message))
		}
		if err := store.Append(ctx, fresh); err != nil {
			log.Printf("goddard: no pude guardar el turno de %s: %v", conversation.ID, err)
		}
	}
	var failure error
	switch end.Outcome.Kind {
	case axe.OutcomeFailed:
		failure = errors.New(end.Outcome.Failure)
		s.failed(ctx, conversation.ID, failure)
	case axe.OutcomeCancelled:
		sink.write(map[string]any{"event": "stopped"})
	}
	sink.write(map[string]any{"event": "done"})
	return sink.reply, failure
}

// failed writes the error into the log. It writes it even when the turn was
// cancelled: what the thread shows is the closing of the turn, and a turn that
// dies without one leaves the thread waiting forever.
func (s *Service) failed(ctx context.Context, conversationID string, cause error) {
	log.Printf("goddard: %s: %v", conversationID, cause)
	_, _ = s.write(context.WithoutCancel(ctx), conversationID, map[string]any{"event": "error", "message": cause.Error()})
}

func (s *Service) write(ctx context.Context, conversationID string, event map[string]any) (chat.Event, error) {
	body, err := json.Marshal(event)
	if err != nil {
		return chat.Event{}, err
	}
	return s.Chat.Append(ctx, conversationID, body)
}

// tools is what the agent can do: the harness' own, plus the memory, the
// skills and the agenda of the project this conversation belongs to, and the
// way to show a file in this thread.
func (s *Service) tools(project, conversationID string) []axe.Tool {
	tools := axe.BuildToolsOn(s.Machine)
	tools = append(tools, memo.Tool(s.Memo), skill.Tool(s.Skill, s.Viewer), schedule.Tool(s.Schedule, s.Viewer.User, project), s.sendTool(conversationID))
	return tools
}

// system is the prompt: what the harness says about its tools, the fragments
// of goddard, the memory of the project and where this turn is running.
func (s *Service) system(ctx context.Context, tools []axe.Tool, conversation chat.Conversation, project string) string {
	out := axe.SystemPrompt(tools)
	skills, err := s.Skill.Index(ctx, s.Viewer)
	if err != nil {
		log.Printf("goddard: no pude leer las skills: %v", err)
	}
	fragments, err := prompt.Assemble(s.Language, s.Spec, []fs.FS{prompt.Builtin}, []prompt.Var{
		{Name: "usuario", Value: s.User},
		{Name: "asistente", Value: s.Assistant},
		{Name: "skills", Value: skills},
	})
	if err != nil {
		log.Printf("goddard: no pude armar el prompt: %v", err)
		return out + "\n" + s.context(conversation)
	}
	out += "\n\n" + fragments
	memory, err := s.Memo.Render(ctx, project)
	if err != nil {
		log.Printf("goddard: no pude leer la memoria: %v", err)
	}
	if strings.TrimSpace(memory) != "" {
		out += "\n\n## Memoria en contexto\n\n" + memory
	}
	return out + "\n" + s.context(conversation)
}

func (s *Service) context(conversation chat.Conversation) string {
	return fmt.Sprintf("## Entorno de ejecución\n- Modelo: %s\n- Workspace: %s\n- Conversación: %s\n",
		s.Model, s.Workspace, conversation.ID)
}

// logSink turns what axe does into the events the web reads: what is worth
// keeping goes to the log, and nothing else. The deltas of a message in flight
// are not kept — the log would be one line per token — so what the thread
// shows afterwards is the message, not how it was typed.
type logSink struct {
	axe.SinkBase
	service        *Service
	conversationID string
	reply          string
}

func (l *logSink) write(event map[string]any) {
	if _, err := l.service.write(context.Background(), l.conversationID, event); err != nil {
		log.Printf("goddard: no pude escribir en el log de %s: %v", l.conversationID, err)
	}
}

// AssistantDelta is the message being written. It is not stored: the log
// would be one line per token. It goes to whoever is watching the
// conversation right now, which is where a half-written sentence belongs.
func (l *logSink) AssistantDelta(text string) {
	body, err := json.Marshal(map[string]any{"event": "delta", "text": text})
	if err != nil {
		return
	}
	l.service.Hub.tell(l.conversationID, body)
}

func (l *logSink) ToolStart(call axe.ToolCall) {
	l.write(map[string]any{"event": "tool_start", "id": call.ID, "name": call.Name, "args": call.Arguments})
}

func (l *logSink) ToolResult(call axe.ToolCall, output axe.ToolOutput, elapsed time.Duration) {
	l.write(map[string]any{
		"event": "tool_result",
		"id":    call.ID,
		"name":  call.Name,
		"text":  output.Text,
		"ms":    elapsed.Milliseconds(),
	})
}

func (l *logSink) Assistant(turn int, message axe.Message, usage axe.Usage) {
	if strings.TrimSpace(message.Content) == "" {
		return
	}
	l.reply = message.Content
	l.write(map[string]any{"event": "assistant", "text": message.Content})
}

// titleOf names a conversation after its first message, the way jimmy did.
func titleOf(text string) string {
	line := strings.TrimSpace(strings.SplitN(text, "\n", 2)[0])
	if line == "" {
		return chat.NewTitle
	}
	words := strings.Fields(line)
	if len(words) > 8 {
		words = words[:8]
	}
	title := strings.Join(words, " ")
	if len([]rune(title)) > 60 {
		title = string([]rune(title)[:60])
	}
	return title
}

// projectSlug is the name the rest of goddard knows the project by: the memory
// and the agenda of a project live under it.
func (s *Service) projectSlug(ctx context.Context, conversation chat.Conversation) string {
	project, ok, err := s.Chat.Project(ctx, conversation.ProjectID)
	if err != nil {
		log.Printf("goddard: no pude leer el proyecto de %s: %v", conversation.ID, err)
		return ""
	}
	if !ok {
		return ""
	}
	return project.Slug
}

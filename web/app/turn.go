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

	"github.com/3-lines-studio/goddard/auth"
	"github.com/3-lines-studio/goddard/axe"
	"github.com/3-lines-studio/goddard/chat"
	"github.com/3-lines-studio/goddard/compute"
	"github.com/3-lines-studio/goddard/memo"
	"github.com/3-lines-studio/goddard/prompt"
	"github.com/3-lines-studio/goddard/schedule"
	"github.com/3-lines-studio/goddard/skill"
	"github.com/3-lines-studio/goddard/workspace"
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
	ErrNotYours       = errors.New("esa conversación no es tuya")
)

// Say writes the message and sends the agent after it. It returns as soon as
// the turn is claimed: what the agent answers shows up in the log, and the web
// reads it from there. The uploads are the attachments of this message, by id:
// they are written to the workspace so the agent can read them, and into the
// log so the thread shows them. The user is who is asking, and the turn keeps
// it: the skills it sees, the agenda it writes and the name in the prompt are
// that person's.
func (s *Service) Say(ctx context.Context, conversationID string, user auth.User, text string, uploads []string) error {
	return s.say(ctx, conversationID, s.whoOf(ctx, user), text, uploads)
}

// say is the same turn for whoever it runs as, which is a person from the web
// and a person or an organization from the agenda.
func (s *Service) say(ctx context.Context, conversationID string, actor who, text string, uploads []string) error {
	conversation, ok, err := s.Chat.Conversation(ctx, conversationID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNoConversation
	}
	if err := s.authorize(ctx, actor.viewer, conversation); err != nil {
		return err
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
	project, _ := s.projectOf(ctx, conversation)
	machine, _, err := s.machine(ctx, project)
	if err != nil {
		_ = s.Chat.Release(ctx, conversationID)
		return err
	}
	if strings.TrimSpace(text) != "" {
		if _, err := s.write(ctx, conversationID, map[string]any{"event": "user", "text": text}); err != nil {
			_ = s.Chat.Release(ctx, conversationID)
			return err
		}
	}
	files, err := s.attach(ctx, conversation, machine, uploads)
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
		_, _ = s.answer(turn, conversation, project, actor, messageOf(text, files))
	}()
	return nil
}

// who is who a turn runs as: the skills and the agenda it sees, and the name
// the prompt is told about. A turn of the web is a person; one of the agenda
// can be an organization, which has no email and no name of its own.
type who struct {
	viewer skill.Viewer
	name   string
}

// whoOf is a person: the skills of their organizations and their own, and the
// name they came in with.
func (s *Service) whoOf(ctx context.Context, user auth.User) who {
	return who{viewer: s.Viewer(ctx, user), name: user.Name}
}

// whoOfOrg is an organization: the skills of the team, and the name of the
// organization in the prompt.
func (s *Service) whoOfOrg(ctx context.Context, orgID string) who {
	name := "la organización " + orgID
	if one, ok, err := s.Orgs.Org(ctx, orgID); err == nil && ok {
		name = one.Name
	}
	return who{viewer: skill.Viewer{Orgs: []string{orgID}}, name: name}
}

// userOf is a person by the id the database minted.
func (s *Service) userOf(ctx context.Context, id string) (auth.User, error) {
	user, ok, err := s.Auth.ByID(ctx, id)
	if err != nil {
		return auth.User{}, err
	}
	if !ok {
		return auth.User{}, fmt.Errorf("el usuario %q no existe", id)
	}
	return user, nil
}

// scopeOf is the memory a turn reads and writes: the general facts of the
// person asking, and the ones of the project, which belong to whoever owns it.
func (s *Service) scopeOf(project chat.Project, userID string) memo.Scope {
	return memo.Scope{
		User:    memo.Owner{Kind: memo.KindUser, ID: userID},
		Project: memo.Owner{Kind: project.Owner.Kind, ID: project.Owner.ID},
		Slug:    project.Slug,
	}
}

// Scope is whose memory a request is about: the person asking, and the project
// by slug when it is one they can see. A project of somebody else is not found,
// which is how the page ends up showing what the prompt shows.
func (s *Service) Scope(ctx context.Context, user auth.User, slug string) (memo.Scope, bool) {
	if slug == "" {
		return s.scopeOf(chat.Project{}, user.ID), true
	}
	projects, err := s.Chat.Projects(ctx, chat.Owner{Kind: chat.OwnerUser, ID: user.ID}, s.viewerOrgs(ctx, user.ID))
	if err != nil {
		log.Printf("goddard: no pude leer los proyectos de %s: %v", user.ID, err)
		return memo.Scope{}, false
	}
	for _, project := range projects {
		if project.Slug == slug {
			return s.scopeOf(project, user.ID), true
		}
	}
	return memo.Scope{}, false
}

// chatOwner is the owner of a project the turn runs under. A task of nobody is
// a project of nobody: it comes out with an owner of nothing, which is what the
// list answers when there is nothing to see.
func (s *Service) chatOwner(userID string) chat.Owner {
	if userID == "" {
		return chat.Owner{Kind: "", ID: ""}
	}
	return chat.Owner{Kind: chat.OwnerUser, ID: userID}
}

// viewerOrgs is the ids of the organizations the user is in, for the lookup of
// what they can see.
func (s *Service) viewerOrgs(ctx context.Context, userID string) []string {
	if userID == "" {
		return nil
	}
	ids, err := s.OrgsOf(ctx, userID)
	if err != nil {
		log.Printf("goddard: no pude leer las organizaciones de %s: %v", userID, err)
		return nil
	}
	return ids
}

// AgendaOf is who is asking for the agenda: the person and the organizations
// they are in, so the tasks of a shared project are in the same list.
func (s *Service) AgendaOf(ctx context.Context, user auth.User) schedule.Viewer {
	return schedule.Viewer{User: user.ID, Orgs: s.viewerOrgs(ctx, user.ID)}
}

// OrgsOf is the ids of the organizations this user is in, or nothing when it
// goes wrong: a project of an organization nobody can read is worse than an
// empty list.
func (s *Service) OrgsOf(ctx context.Context, userID string) ([]string, error) {
	orgs, err := s.Orgs.Orgs(ctx, userID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(orgs))
	for _, one := range orgs {
		ids = append(ids, one.ID)
	}
	return ids, nil
}

// authorize is who may use a conversation: whoever created it, or anybody in
// the organization its project belongs to. It runs before anything is written,
// so what it refuses never reaches the log.
func (s *Service) authorize(ctx context.Context, viewer skill.Viewer, conversation chat.Conversation) error {
	if conversation.CreatedBy == viewer.User {
		return nil
	}
	project, ok, err := s.Chat.Project(ctx, conversation.ProjectID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotYours
	}
	if project.Owner.Kind == chat.OwnerOrg {
		_, in, err := s.Orgs.Role(ctx, project.Owner.ID, viewer.User)
		if err != nil {
			return err
		}
		if in {
			return nil
		}
	}
	return ErrNotYours
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
func (s *Service) attach(ctx context.Context, conversation chat.Conversation, machine axe.Machine, uploads []string) ([]attached, error) {
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
		if err := machine.Write(path, upload.Bytes); err != nil {
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
func (s *Service) answer(ctx context.Context, conversation chat.Conversation, project chat.Project, actor who, text string) (string, error) {
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
	machine, dir, err := s.machine(ctx, project)
	if err != nil {
		s.failed(ctx, conversation.ID, err)
		return "", err
	}
	provider, model, err := s.ModelOf(ctx, project.Owner)
	if err != nil {
		s.failed(ctx, conversation.ID, err)
		return "", err
	}
	tools := s.tools(project, conversation.ID, machine, actor)
	options := &axe.RunOptions{
		Model:    model,
		System:   s.system(ctx, tools, conversation, project, dir, model, actor),
		Tools:    tools,
		MaxTurns: math.MaxInt,
	}
	end := axe.RunStream(ctx, provider, options, messages, sink)
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
func (s *Service) tools(project chat.Project, conversationID string, machine axe.Machine, actor who) []axe.Tool {
	tools := axe.BuildToolsOn(machine)
	owner := schedule.Owner{Kind: project.Owner.Kind, ID: project.Owner.ID}
	agenda := schedule.Viewer{User: actor.viewer.User, Orgs: actor.viewer.Orgs}
	tools = append(tools, memo.Tool(s.Memo, s.scopeOf(project, actor.viewer.User)), skill.Tool(s.Skill, actor.viewer), schedule.Tool(s.Schedule, agenda, owner, project.Slug), s.sendTool(conversationID, machine))
	return tools
}

// system is the prompt: what the harness says about its tools, the fragments
// of goddard, the memory of the project and where this turn is running.
func (s *Service) system(ctx context.Context, tools []axe.Tool, conversation chat.Conversation, project chat.Project, dir, model string, actor who) string {
	out := axe.SystemPrompt(tools)
	skills, err := s.Skill.Index(ctx, actor.viewer)
	if err != nil {
		log.Printf("goddard: no pude leer las skills: %v", err)
	}
	fragments, err := prompt.Assemble(s.Language, s.Spec, []fs.FS{prompt.Builtin}, []prompt.Var{
		{Name: "usuario", Value: actor.name},
		{Name: "asistente", Value: s.Assistant},
		{Name: "skills", Value: skills},
	})
	if err != nil {
		log.Printf("goddard: no pude armar el prompt: %v", err)
		return out + "\n" + s.context(conversation, dir, model)
	}
	out += "\n\n" + fragments
	memory, err := s.Memo.Render(ctx, s.scopeOf(project, actor.viewer.User))
	if err != nil {
		log.Printf("goddard: no pude leer la memoria: %v", err)
	}
	if strings.TrimSpace(memory) != "" {
		out += "\n\n## Memoria en contexto\n\n" + memory
	}
	return out + "\n" + s.context(conversation, dir, model)
}

func (s *Service) context(conversation chat.Conversation, dir, model string) string {
	return fmt.Sprintf("## Entorno de ejecución\n- Modelo: %s\n- Workspace: %s\n- Conversación: %s\n",
		model, dir, conversation.ID)
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

// projectOf is the project a conversation belongs to.
func (s *Service) projectOf(ctx context.Context, conversation chat.Conversation) (chat.Project, bool) {
	project, ok, err := s.Chat.Project(ctx, conversation.ProjectID)
	if err != nil {
		log.Printf("goddard: no pude leer el proyecto de %s: %v", conversation.ID, err)
		return chat.Project{}, false
	}
	return project, ok
}

// machine is where the tools of a turn run: the machine of the owner of the
// project and the directory of the project inside the volume that machine
// mounts. A turn runs in the sandbox of its owner and never in the container
// goddard runs in, which carries no tools.
func (s *Service) machine(ctx context.Context, project chat.Project) (axe.Machine, string, error) {
	space, channel, err := s.machineOf(ctx, project.Owner)
	if err != nil {
		return nil, "", err
	}
	dir := space.ProjectDir(project.Slug)
	return compute.NewMachine(channel, dir), dir, nil
}

// machineOf is the machine of an owner: the channel into its sandbox and the
// workspace where its projects live. The volume is checked here and not when a
// sandbox is loaded: a sandbox comes with it mounted, and a turn that runs
// without one would write where nothing lasts — a directory that looks like the
// volume and goes away with the machine.
func (s *Service) machineOf(ctx context.Context, owner chat.Owner) (workspace.Workspace, compute.Channel, error) {
	space, err := s.Workspace(ctx, owner)
	if err != nil {
		return workspace.Workspace{}, nil, err
	}
	sandbox, err := s.Sandbox(ctx, owner)
	if err != nil {
		return workspace.Workspace{}, nil, err
	}
	if err := sandbox.Ready(); err != nil {
		return workspace.Workspace{}, nil, err
	}
	channel := s.Dialer.Dial(sandbox)
	mounted, err := compute.NewMachine(channel, space.Path).Mounted(ctx)
	if err != nil {
		return workspace.Workspace{}, nil, fmt.Errorf("no pude entrar a la máquina: %w", err)
	}
	if !mounted {
		return workspace.Workspace{}, nil, fmt.Errorf("la máquina no tiene el volumen de %s montado en %s", owner.ID, space.Path)
	}
	return space, channel, nil
}

// ProjectDir is the directory of a project: the workspace of whoever owns it —
// a person, or an organization — and the slug of the project inside it.
func (s *Service) ProjectDir(ctx context.Context, project chat.Project) (string, error) {
	space, err := s.Workspace(ctx, project.Owner)
	if err != nil {
		return "", err
	}
	return space.ProjectDir(project.Slug), nil
}

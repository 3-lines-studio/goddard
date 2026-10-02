package app_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/3-lines-studio/goddard/chat"
	"github.com/3-lines-studio/goddard/compute/sshtest"
	"github.com/3-lines-studio/goddard/web/app"
	"github.com/3-lines-studio/goddard/web/app/apptest"
)

// TestATurnRunsInTheSandboxOverSSH is the whole way of production: the app
// reads the workspace of the owner and the three secrets of its sandbox, dials
// the machine over ssh and the tools run there. What the agent writes lands
// where that machine has the volume, inside the directory of the project and
// not at the root of the volume, and nothing of the turn lands in the volume
// the app has at hand — which, in a test, is a directory of this same machine,
// so it stays empty.
func TestATurnRunsInTheSandboxOverSSH(t *testing.T) {
	service := apptest.Service(t, apptest.Provider(t,
		[]string{
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","function":{"name":"bash","arguments":"{\"command\":\"echo hola > nota.txt\"}"}}]}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		},
		[]string{
			`{"choices":[{"delta":{"content":"listo"}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		}))
	user := apptest.User(t, service)
	conversation := apptest.Thread(t, service)

	server := sshtest.New(t)
	volume := t.TempDir()
	owner := chat.Owner{Kind: chat.OwnerUser, ID: user.ID}
	if err := service.SaveWorkspace(t.Context(), owner, volume, app.Sandbox{
		Addr: server.Addr,
		User: server.User,
		Key:  server.Key,
	}, user.ID); err != nil {
		t.Fatalf("save: %v", err)
	}
	service.Dialer = app.SSH{}

	upload, err := service.Chat.PutUpload(t.Context(), conversation.ID, "adjunto.txt", "text/plain", []byte("lo que subí"))
	if err != nil {
		t.Fatalf("putUpload: %v", err)
	}
	if err := service.Say(t.Context(), conversation.ID, user, "guardá la nota", []string{upload.ID}); err != nil {
		t.Fatalf("say: %v", err)
	}
	apptest.Wait(t, service, conversation.ID)

	dir := filepath.Join(volume, "goddard")
	for path, want := range map[string]string{
		filepath.Join(dir, "nota.txt"):                              "hola\n",
		filepath.Join(dir, "files", conversation.ID, "adjunto.txt"): "lo que subí",
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("el sandbox no tiene %s: %v", path, err)
		}
		if string(data) != want {
			t.Fatalf("%s quedó %q", path, data)
		}
	}
	if atRoot, err := os.ReadDir(volume); err != nil {
		t.Fatalf("no pude mirar el volumen del sandbox: %v", err)
	} else if len(atRoot) != 1 || atRoot[0].Name() != "goddard" {
		t.Fatalf("el volumen del sandbox quedó con %v: el turno escribe en el proyecto, no en la raíz", atRoot)
	}
	if stray, err := os.ReadDir(filepath.Join(service.Volumes, user.ID)); err != nil {
		t.Fatalf("no pude mirar el volumen que goddard tiene a mano: %v", err)
	} else if len(stray) > 0 {
		t.Fatalf("el turno también escribió en el volumen que goddard tiene a mano: %v", stray)
	}
}

// projectDir is the directory of a project: the workspace of the owner inside
// the volume, and the slug of the project, which is where the tools of a turn
// run.
func projectDir(t *testing.T, service *app.Service, ownerID, slug string) string {
	t.Helper()
	dir, err := service.ProjectDir(t.Context(), chat.Project{
		Slug:  slug,
		Owner: chat.Owner{Kind: chat.OwnerUser, ID: ownerID},
	})
	if err != nil {
		t.Fatalf("project dir: %v", err)
	}
	return dir
}

func TestAnAttachmentLandsInTheWorkspaceOfItsProject(t *testing.T) {
	service := apptest.Service(t, apptest.Provider(t, []string{
		`{"choices":[{"delta":{"content":"la leí"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	}))
	user := apptest.User(t, service)
	conversation := apptest.Thread(t, service)
	upload, err := service.Chat.PutUpload(t.Context(), conversation.ID, "nota.txt", "text/plain", []byte("hola"))
	if err != nil {
		t.Fatalf("putUpload: %v", err)
	}
	if err := service.Say(t.Context(), conversation.ID, user, "miralo", []string{upload.ID}); err != nil {
		t.Fatalf("say: %v", err)
	}
	apptest.Wait(t, service, conversation.ID)

	path := filepath.Join(projectDir(t, service, user.ID, "goddard"), "files", conversation.ID, "nota.txt")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("el adjunto no quedó en %s: %v", path, err)
	}
	if string(data) != "hola" {
		t.Fatalf("el adjunto quedó %q", data)
	}
	if _, err := os.Stat(filepath.Join(service.Volumes, "files", conversation.ID, "nota.txt")); !os.IsNotExist(err) {
		t.Fatalf("el adjunto también fue a la raíz del volumen: %v", err)
	}
}

func TestTheFilesOfTwoProjectsDoNotMix(t *testing.T) {
	service := apptest.Service(t, apptest.Provider(t,
		[]string{
			`{"choices":[{"delta":{"content":"la primera"}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		},
		[]string{
			`{"choices":[{"delta":{"content":"la segunda"}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		}))
	user := apptest.User(t, service)
	one := apptest.Thread(t, service)
	other, err := service.Chat.CreateProject(t.Context(), "otro", chat.Owner{Kind: chat.OwnerUser, ID: user.ID}, user.ID)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	two, err := service.Chat.CreateConversation(t.Context(), other.ID, "", "", user.ID)
	if err != nil {
		t.Fatalf("conversation: %v", err)
	}
	first, err := service.Chat.PutUpload(t.Context(), one.ID, "nota.txt", "text/plain", []byte("uno"))
	if err != nil {
		t.Fatalf("putUpload: %v", err)
	}
	second, err := service.Chat.PutUpload(t.Context(), two.ID, "nota.txt", "text/plain", []byte("dos"))
	if err != nil {
		t.Fatalf("putUpload: %v", err)
	}
	for conversation, upload := range map[string]string{one.ID: first.ID, two.ID: second.ID} {
		if err := service.Say(t.Context(), conversation, user, "miralo", []string{upload}); err != nil {
			t.Fatalf("say en %s: %v", conversation, err)
		}
		apptest.Wait(t, service, conversation)
	}

	for path, want := range map[string]string{
		filepath.Join(projectDir(t, service, user.ID, "goddard"), "files", one.ID, "nota.txt"): "uno",
		filepath.Join(projectDir(t, service, user.ID, "otro"), "files", two.ID, "nota.txt"):    "dos",
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("no está %s: %v", path, err)
		}
		if string(data) != want {
			t.Fatalf("%s quedó %q", path, data)
		}
	}
}

func TestTheAgentShowsAFileOfItsOwnWorkspace(t *testing.T) {
	service := apptest.Service(t, apptest.Provider(t,
		[]string{
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","function":{"name":"send","arguments":"{\"path\":\"grafico.png\"}"}}]}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		},
		[]string{
			`{"choices":[{"delta":{"content":"ahí va"}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		}))
	user := apptest.User(t, service)
	image := append([]byte("\x89PNG\r\n\x1a\n"), []byte("lo que sea el resto")...)
	dir := projectDir(t, service, user.ID, "goddard")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("no pude armar %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "grafico.png"), image, 0o644); err != nil {
		t.Fatalf("no pude escribir la imagen: %v", err)
	}
	conversation := apptest.Thread(t, service)
	if err := service.Say(t.Context(), conversation.ID, user, "mostrame el gráfico", nil); err != nil {
		t.Fatalf("say: %v", err)
	}
	apptest.Wait(t, service, conversation.ID)

	var id string
	for _, body := range apptest.Events(t, service, conversation.ID) {
		if event := apptest.Event(t, body); event["event"] == "file" {
			id, _ = event["id"].(string)
		}
	}
	if id == "" {
		t.Fatal("el archivo no llegó al hilo")
	}
	upload, ok, err := service.Chat.Upload(t.Context(), id)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if !ok || string(upload.Bytes) != string(image) {
		t.Fatalf("los bytes no son los del workspace")
	}
}

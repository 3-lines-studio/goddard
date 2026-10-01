package app_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/3-lines-studio/goddard/axe"
	"github.com/3-lines-studio/goddard/web/app/apptest"
)

type memoryMachine struct {
	files  map[string][]byte
	reads  []string
	writes []string
}

func newMemoryMachine() *memoryMachine {
	return &memoryMachine{files: map[string][]byte{}}
}

func (m *memoryMachine) Read(path string) ([]byte, error) {
	m.reads = append(m.reads, path)
	data, ok := m.files[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return data, nil
}

func (m *memoryMachine) Write(path string, bytes []byte) error {
	m.writes = append(m.writes, path)
	m.files[path] = bytes
	return nil
}

func (m *memoryMachine) Stat(path string) (axe.MachineEntry, error) {
	data, ok := m.files[path]
	if !ok {
		return axe.MachineEntry{}, os.ErrNotExist
	}
	return axe.MachineEntry{Name: filepath.Base(path), Size: uint64(len(data))}, nil
}

func (m *memoryMachine) List(string) ([]axe.MachineEntry, error) { return nil, nil }

func (m *memoryMachine) Remove(path string) error {
	delete(m.files, path)
	return nil
}

func (m *memoryMachine) Run(string, uint64, axe.Progress) string { return "" }

func TestAnAttachmentGoesThroughTheMachine(t *testing.T) {
	service := apptest.Service(t, apptest.Provider(t, []string{
		`{"choices":[{"delta":{"content":"la leí"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	}))
	machine := newMemoryMachine()
	service.Machine = machine
	conversation := apptest.Thread(t, service)
	upload, err := service.Chat.PutUpload(t.Context(), conversation.ID, "nota.txt", "text/plain", []byte("hola"))
	if err != nil {
		t.Fatalf("putUpload: %v", err)
	}
	if err := service.Say(t.Context(), conversation.ID, "miralo", []string{upload.ID}); err != nil {
		t.Fatalf("say: %v", err)
	}
	apptest.Wait(t, service, conversation.ID)

	path := filepath.Join("files", conversation.ID, "nota.txt")
	if string(machine.files[path]) != "hola" {
		t.Fatalf("el adjunto quedó %v", machine.files)
	}
	if _, err := os.Stat(filepath.Join(service.Workspace, path)); !os.IsNotExist(err) {
		t.Fatalf("el adjunto también fue al disco local: %v", err)
	}
}

func TestTheAgentShowsAFileThroughTheMachine(t *testing.T) {
	service := apptest.Service(t, apptest.Provider(t,
		[]string{
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","function":{"name":"send","arguments":"{\"path\":\"grafico.png\"}"}}]}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		},
		[]string{
			`{"choices":[{"delta":{"content":"ahí va"}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		}))
	machine := newMemoryMachine()
	image := append([]byte("\x89PNG\r\n\x1a\n"), []byte("lo que sea el resto")...)
	machine.files["grafico.png"] = image
	service.Machine = machine
	conversation := apptest.Thread(t, service)
	if err := service.Say(t.Context(), conversation.ID, "mostrame el gráfico", nil); err != nil {
		t.Fatalf("say: %v", err)
	}
	apptest.Wait(t, service, conversation.ID)

	if len(machine.reads) == 0 || machine.reads[0] != "grafico.png" {
		t.Fatalf("send no leyó del machine: %v", machine.reads)
	}
	var id string
	for _, body := range apptest.Events(t, service, conversation.ID) {
		event := apptest.Event(t, body)
		if event["event"] == "file" {
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
		t.Fatalf("los bytes no son los del machine")
	}
}

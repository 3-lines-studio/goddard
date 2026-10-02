package compute

import (
	"bytes"
	"strings"
	"testing"

	"github.com/3-lines-studio/goddard/compute/sshtest"
)

func newTestSSH(t *testing.T) *SSH {
	t.Helper()
	server := sshtest.New(t)
	return NewSSH(server.Addr, server.User, server.Key)
}

func TestTheSSHChannelCarriesWhatTheMachineAsks(t *testing.T) {
	root := t.TempDir()
	machine := NewMachine(newTestSSH(t), root)

	content := []byte{0, 1, 2, '\n', 'h', 'o', 'l', 'a', 0}
	if err := machine.Write("dir/nota.bin", content); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := machine.Read("dir/nota.bin")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("leyó %q", got)
	}
	entry, err := machine.Stat("dir/nota.bin")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if entry.Name != "nota.bin" || entry.IsDir || entry.Size != uint64(len(content)) {
		t.Fatalf("midió %+v", entry)
	}
	entries, err := machine.List("")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "dir" || !entries[0].IsDir {
		t.Fatalf("listó %+v", entries)
	}
	if out := machine.Run("pwd", 10, nil); strings.TrimSpace(out) != root {
		t.Fatalf("pwd dio %q", out)
	}
	if out := machine.Run("exit 3", 10, nil); !strings.Contains(out, "exit status 3") {
		t.Fatalf("el run quedó %q", out)
	}
	if err := machine.Remove("dir"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := machine.Stat("dir"); err == nil {
		t.Fatal("el directorio sigue ahí")
	}
}

func TestTheSSHChannelCutsTheCommandThatTakesTooLong(t *testing.T) {
	machine := NewMachine(newTestSSH(t), t.TempDir())
	out := machine.Run("sleep 5", 1, nil)
	if !strings.Contains(out, "error: command timed out after 1 seconds") {
		t.Fatalf("el run quedó %q", out)
	}
}

func TestTheSSHChannelSaysWhenTheKeyIsNotAKey(t *testing.T) {
	machine := NewMachine(NewSSH("127.0.0.1:1", "tester", []byte("no soy una llave")), t.TempDir())
	if _, err := machine.Read("nada"); err == nil {
		t.Fatal("leyó con una llave que no es una llave")
	}
}

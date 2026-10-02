package compute

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/3-lines-studio/goddard/axe"
)

func newTestMachine(t *testing.T) *Machine {
	t.Helper()
	return NewMachine(Shell{}, t.TempDir())
}

func TestTheMachineWritesAndReadsAFile(t *testing.T) {
	machine := newTestMachine(t)
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
}

func TestTheMachineRewritesWhatItAlreadyWrote(t *testing.T) {
	machine := newTestMachine(t)
	if err := machine.Write("nota.txt", []byte("largo y viejo")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := machine.Write("nota.txt", []byte("corto")); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := machine.Read("nota.txt")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "corto" {
		t.Fatalf("quedó %q", got)
	}
}

func TestTheMachineReadsWhatIsNotThere(t *testing.T) {
	machine := newTestMachine(t)
	if _, err := machine.Read("no-existe.txt"); err == nil {
		t.Fatal("leyó lo que no está")
	}
}

func TestTheMachineStatsAFileADirectoryAndSomethingMissing(t *testing.T) {
	machine := newTestMachine(t)
	if err := machine.Write("dir/nota.txt", []byte("hola")); err != nil {
		t.Fatalf("write: %v", err)
	}

	file, err := machine.Stat("dir/nota.txt")
	if err != nil {
		t.Fatalf("stat del archivo: %v", err)
	}
	if file.Name != "nota.txt" || file.IsDir || file.Size != 4 {
		t.Fatalf("el archivo quedó %+v", file)
	}

	dir, err := machine.Stat("dir")
	if err != nil {
		t.Fatalf("stat del directorio: %v", err)
	}
	if !dir.IsDir {
		t.Fatalf("el directorio quedó %+v", dir)
	}

	if _, err := machine.Stat("no-existe.txt"); err == nil {
		t.Fatal("midió lo que no está")
	}
}

func TestTheMachineListsADirectory(t *testing.T) {
	machine := newTestMachine(t)
	if err := machine.Write("b.txt", []byte("bb")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := machine.Write("a.txt", []byte("a")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := machine.Write("dir/adentro.txt", []byte("x")); err != nil {
		t.Fatalf("write: %v", err)
	}

	entries, err := machine.List("")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("listó %+v", entries)
	}
	if entries[0].Name != "a.txt" || entries[0].Size != 1 || entries[0].IsDir {
		t.Fatalf("el primero quedó %+v", entries[0])
	}
	if entries[2].Name != "dir" || !entries[2].IsDir {
		t.Fatalf("el último quedó %+v", entries[2])
	}
}

func TestTheMachineRemovesAFileAndADirectory(t *testing.T) {
	machine := newTestMachine(t)
	if err := machine.Write("dir/adentro.txt", []byte("x")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := machine.Remove("dir"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := machine.Stat("dir"); err == nil {
		t.Fatal("el directorio sigue ahí")
	}
}

func TestTheMachineRunsInItsDirectory(t *testing.T) {
	root := t.TempDir()
	machine := NewMachine(Shell{}, root)
	out := machine.Run("pwd", 10, nil)
	if strings.TrimSpace(out) != root {
		t.Fatalf("pwd dio %q", out)
	}
}

func TestTheMachineSaysTheExitStatus(t *testing.T) {
	machine := newTestMachine(t)
	out := machine.Run("echo antes; exit 3", 10, nil)
	if !strings.Contains(out, "antes") || !strings.Contains(out, "exit status 3") {
		t.Fatalf("el run quedó %q", out)
	}
}

func TestTheMachineTruncatesTheLongOutput(t *testing.T) {
	machine := newTestMachine(t)
	out := machine.Run("seq 1 20000", 10, nil)
	if !strings.Contains(out, "[Output truncated to the last 16KB.]") {
		t.Fatalf("el run quedó %q", out)
	}
	if !strings.Contains(out, "\n20000\n") {
		t.Fatalf("no se quedó con la cola: %q", out[len(out)-80:])
	}
	if strings.Contains(out, "1\n2\n3\n") {
		t.Fatalf("se quedó con el principio: %q", out[:40])
	}
}

func TestTheMachineCutsTheCommandThatTakesTooLong(t *testing.T) {
	machine := newTestMachine(t)
	out := machine.Run("sleep 30", 1, nil)
	if !strings.Contains(out, "error: command timed out after 1 seconds") {
		t.Fatalf("el run quedó %q", out)
	}
}

func TestTheMachineAndTheLocalOneSeeTheSameFiles(t *testing.T) {
	root := t.TempDir()
	local := axe.NewLocal(root)
	remote := NewMachine(Shell{}, root)

	if err := local.Write("desde-local.txt", []byte("hola")); err != nil {
		t.Fatalf("write local: %v", err)
	}
	got, err := remote.Read("desde-local.txt")
	if err != nil {
		t.Fatalf("read remoto: %v", err)
	}
	if string(got) != "hola" {
		t.Fatalf("el remoto leyó %q", got)
	}
	if err := remote.Write("desde-remoto.txt", []byte("chau")); err != nil {
		t.Fatalf("write remoto: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "desde-remoto.txt")); err != nil {
		t.Fatalf("el local no ve el archivo del remoto: %v", err)
	}

	entry, err := remote.Stat("desde-local.txt")
	if err != nil {
		t.Fatalf("stat remoto: %v", err)
	}
	ones, err := local.List("")
	if err != nil {
		t.Fatalf("list local: %v", err)
	}
	names := []string{}
	for _, one := range ones {
		names = append(names, one.Name)
	}
	if entry.Size != 4 || len(names) != 2 {
		t.Fatalf("el remoto vio %+v y el local %v", entry, names)
	}

	if got, want := remote.Run("echo hola", 10, nil), local.Run("echo hola", 10, nil); got != want {
		t.Fatalf("el run remoto dio %q y el local %q", got, want)
	}
}

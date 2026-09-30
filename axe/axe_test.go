package axe

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAtomicWriteRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")

	if err := AtomicWrite(path, []byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := readFile(t, path); got != "hello" {
		t.Fatalf("contenido: %q", got)
	}

	if err := os.Chmod(path, 0o751); err != nil {
		t.Fatal(err)
	}
	if err := AtomicWrite(path, []byte("world!")); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if got := readFile(t, path); got != "world!" {
		t.Fatalf("contenido: %q", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o751 {
		t.Fatalf("permisos: %o", info.Mode().Perm())
	}
}

func TestAtomicWriteReplacesSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	link := filepath.Join(dir, "link.txt")
	if err := os.WriteFile(target, []byte("target"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	if err := AtomicWrite(link, []byte("replacement")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := readFile(t, target); got != "target" {
		t.Fatalf("target: %q", got)
	}
	if got := readFile(t, link); got != "replacement" {
		t.Fatalf("link: %q", got)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("el symlink sobrevivio")
	}
}

func TestAtomicWriteCreatesParents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c.txt")
	if err := AtomicWrite(path, []byte("x")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := readFile(t, path); got != "x" {
		t.Fatalf("contenido: %q", got)
	}
}

func TestAtomicWriteSurvivesProcessKill(t *testing.T) {
	if crashPath := os.Getenv("AXE_TEST_CRASH_PATH"); crashPath != "" {
		atomicWriteWith(crashPath, func(file *os.File) error {
			if _, err := file.WriteString("new"); err != nil {
				return err
			}
			marker := filepath.Join(filepath.Dir(crashPath), "ready")
			if err := os.WriteFile(marker, []byte("ready"), 0o644); err != nil {
				return err
			}
			time.Sleep(time.Hour)
			return nil
		})
		return
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	marker := filepath.Join(dir, "ready")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	command := exec.Command(os.Args[0], "-test.run=TestAtomicWriteSurvivesProcessKill")
	command.Env = append(os.Environ(), "AXE_TEST_CRASH_PATH="+path)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	for range 500 {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("el hijo no llego a escribir")
	}
	command.Process.Kill()
	command.Wait()

	if got := readFile(t, path); got != "old" {
		t.Fatalf("el destino quedo a medias: %q", got)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	foundTemp := false
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".axe-tmp-") {
			foundTemp = true
		}
	}
	if !foundTemp {
		t.Fatal("el temp del hijo muerto desaparecio")
	}
}

func TestAtomicWriteConcurrentProcesses(t *testing.T) {
	if payload := os.Getenv("AXE_TEST_CONCURRENT_PAYLOAD"); payload != "" {
		if err := AtomicWrite(os.Getenv("AXE_TEST_CONCURRENT_PATH"), []byte(payload)); err != nil {
			t.Fatal(err)
		}
		return
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	payloads := make([]string, 16)
	commands := make([]*exec.Cmd, 16)
	for index := range payloads {
		payloads[index] = strings.Repeat("writer-"+string(rune('a'+index))+"-", 500)
		command := exec.Command(os.Args[0], "-test.run=TestAtomicWriteConcurrentProcesses")
		command.Env = append(os.Environ(),
			"AXE_TEST_CONCURRENT_PAYLOAD="+payloads[index],
			"AXE_TEST_CONCURRENT_PATH="+path)
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		commands[index] = command
	}
	for _, command := range commands {
		if err := command.Wait(); err != nil {
			t.Fatalf("un escritor fallo: %v", err)
		}
	}

	written := readFile(t, path)
	matched := false
	for _, candidate := range payloads {
		if candidate == written {
			matched = true
		}
	}
	if !matched {
		t.Fatalf("el destino no es el payload de ningun escritor (%d bytes)", len(written))
	}
}

func TestTempNameDistinguishesProcessesAndCalls(t *testing.T) {
	if tempName(1, 100, 7, 0) == tempName(2, 100, 7, 0) {
		t.Fatal("dos procesos eligieron el mismo temp")
	}
	if tempName(1, 100, 7, 0) == tempName(1, 100, 7, 1) {
		t.Fatal("dos llamadas eligieron el mismo temp")
	}
	if tempName(1, 100, 7, 0) == tempName(1, 101, 7, 0) {
		t.Fatal("dos segundos distintos dieron el mismo temp")
	}
	if tempName(1, 100, 7, 0) == tempName(1, 100, 8, 0) {
		t.Fatal("dos nanosegundos distintos dieron el mismo temp")
	}
}

func TestSystemPrompt(t *testing.T) {
	tools := []Tool{
		{Name: "bash", Snippet: "corre comandos"},
		{Name: "sin-snippet"},
		{Name: "read", Snippet: "lee archivos"},
	}
	want := "Available tools:\n- bash: corre comandos\n- read: lee archivos\n"
	if got := SystemPrompt(tools); got != want {
		t.Errorf("\n got:  %q\n want: %q", got, want)
	}
	if got := SystemPrompt(nil); got != "Available tools:\n" {
		t.Errorf("sin tools: %q", got)
	}
}

func TestSetNonDumpable(t *testing.T) {
	if !SetNonDumpable() {
		t.Skip("prctl no disponible")
	}
}

func TestSetNonDumpableBlocksChildEnvironRead(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root lee /proc/<pid>/environ igual")
	}
	hidden := os.Getenv("AXE_TEST_HIDDEN_TOKEN")
	if hidden != "" {
		SetNonDumpable()
		time.Sleep(time.Minute)
		return
	}

	command := exec.Command(os.Args[0], "-test.run=TestSetNonDumpableBlocksChildEnvironRead")
	command.Env = append(os.Environ(), "AXE_TEST_HIDDEN_TOKEN=leaky-value")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		command.Process.Kill()
		command.Wait()
	}()

	environ, err := os.ReadFile("/proc/" + strconv.Itoa(command.Process.Pid) + "/environ")
	if err != nil {
		return
	}
	if strings.Contains(string(environ), "leaky-value") {
		t.Fatal("el environ del hijo se filtro")
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

package axe

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

type fakeMachine struct {
	files map[string][]byte
	runs  []string
}

func newFakeMachine() *fakeMachine {
	return &fakeMachine{files: map[string][]byte{}}
}

func (f *fakeMachine) Read(path string) ([]byte, error) {
	data, ok := f.files[path]
	if !ok {
		return nil, fmt.Errorf("no such file: %s", path)
	}
	return data, nil
}

func (f *fakeMachine) Write(path string, bytes []byte) error {
	f.files[path] = bytes
	return nil
}

func (f *fakeMachine) Stat(path string) (MachineEntry, error) {
	data, ok := f.files[path]
	if !ok {
		return MachineEntry{}, os.ErrNotExist
	}
	return MachineEntry{Name: path, Size: uint64(len(data))}, nil
}

func (f *fakeMachine) List(string) ([]MachineEntry, error) {
	return nil, nil
}

func (f *fakeMachine) Remove(path string) error {
	if _, ok := f.files[path]; !ok {
		return fmt.Errorf("no such file: %s", path)
	}
	delete(f.files, path)
	return nil
}

func (f *fakeMachine) Run(command string, _ uint64, _ Progress) string {
	f.runs = append(f.runs, command)
	return "ran: " + command
}

func TestResolve(t *testing.T) {
	cases := []struct{ dir, path, want string }{
		{"/base", "a.txt", "/base/a.txt"},
		{"/base/", "a.txt", "/base/a.txt"},
		{"/base", "/otro/a.txt", "/otro/a.txt"},
		{"", "a.txt", "a.txt"},
	}
	for _, test := range cases {
		if got := Resolve(test.dir, test.path); got != test.want {
			t.Errorf("Resolve(%q, %q) = %q, want %q", test.dir, test.path, got, test.want)
		}
	}
}

func TestLocalResolvesAgainstItsDirectory(t *testing.T) {
	root := t.TempDir()
	machine := NewLocal(root)

	if err := machine.Write("adentro.txt", []byte("hola")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "adentro.txt")); err != nil {
		t.Fatalf("escribio fuera del directorio: %v", err)
	}
	data, err := machine.Read("adentro.txt")
	if err != nil || string(data) != "hola" {
		t.Fatalf("read: %q %v", data, err)
	}
	entries, err := machine.List("")
	if err != nil || len(entries) != 1 {
		t.Fatalf("list: %v %v", entries, err)
	}
	if entries[0].Name != "adentro.txt" || entries[0].IsDir {
		t.Fatalf("entry: %+v", entries[0])
	}
	if err := machine.Remove("adentro.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := machine.Read("adentro.txt"); err == nil {
		t.Fatal("el archivo sigue ahi")
	}
}

func TestLocalListSortsAndSizes(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "b.txt"), []byte("bb"), 0o644)
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o644)
	os.Mkdir(filepath.Join(root, "dir"), 0o755)

	entries, err := NewLocal(root).List("")
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, entry := range entries {
		names = append(names, entry.Name)
	}
	if strings.Join(names, ",") != "a.txt,b.txt,dir" {
		t.Fatalf("orden: %v", names)
	}
	if entries[0].Size != 1 || entries[1].Size != 2 || !entries[2].IsDir {
		t.Fatalf("tamanos: %+v", entries)
	}
}

func TestLocalRemoveTakesSymlinkNotTarget(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	os.WriteFile(target, []byte("x"), 0o644)
	link := filepath.Join(root, "link.txt")
	os.Symlink(target, link)

	if err := NewLocal(root).Remove("link.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("borro el destino del symlink: %v", err)
	}
	if _, err := os.Lstat(link); err == nil {
		t.Fatal("el symlink sigue ahi")
	}
}

func TestSanitizeStripsControlCharacters(t *testing.T) {
	cases := []struct{ in, want string }{
		{"a\x00b\x1bc\x7fd", "abc\x7fd"},
		{"\x1b[31mred\x1b[0m", "[31mred[0m"},
		{"keep\tnewline\ncr\r", "keep\tnewline\ncr\r"},
		{"\ufff9fmt\ufffb", "fmt"},
		{"emoji 🙈 ok", "emoji 🙈 ok"},
	}
	for _, test := range cases {
		if got := sanitize(test.in); got != test.want {
			t.Errorf("sanitize(%q) = %q, want %q", test.in, got, test.want)
		}
	}
}

func TestReadAcceptsFloatOffset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	read := ReadTool(NewLocal(""))
	out := read.Run(fmt.Sprintf(`{"path":"%s","offset":2.0,"limit":1.0}`, path), nil).Text
	if !strings.HasPrefix(out, "b") {
		t.Fatalf("got: %q", out)
	}
	if strings.Contains(out, "invalid arguments") {
		t.Fatalf("got: %q", out)
	}
}

func TestReadAttachesImagesAndRejectsBinary(t *testing.T) {
	dir := t.TempDir()
	read := ReadTool(NewLocal(""))

	png := filepath.Join(dir, "shot.png")
	os.WriteFile(png, []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0}, 0o644)
	out := read.Run(fmt.Sprintf(`{"path":"%s"}`, png), nil)
	if len(out.Images) != 1 {
		t.Fatalf("text: %q", out.Text)
	}
	if !strings.HasPrefix(out.Images[0].URL, "data:image/png;base64,") {
		t.Fatalf("url: %q", out.Images[0].URL)
	}
	if !strings.Contains(out.Text, "shot.png") {
		t.Fatalf("got: %q", out.Text)
	}

	bin := filepath.Join(dir, "thing.bin")
	os.WriteFile(bin, []byte{0, 1, 2, 3, 0, 4}, 0o644)
	out = read.Run(fmt.Sprintf(`{"path":"%s"}`, bin), nil)
	if len(out.Images) != 0 || !strings.Contains(out.Text, "binary file") {
		t.Fatalf("got: %+v", out)
	}

	txt := filepath.Join(dir, "t.txt")
	os.WriteFile(txt, []byte("hello\n"), 0o644)
	out = read.Run(fmt.Sprintf(`{"path":"%s"}`, txt), nil)
	if len(out.Images) != 0 || out.Text != "hello" {
		t.Fatalf("got: %+v", out)
	}
}

func TestReadOffsetBeyondEnd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	os.WriteFile(path, []byte("a\nb\n"), 0o644)
	read := ReadTool(NewLocal(""))
	out := read.Run(fmt.Sprintf(`{"path":"%s","offset":9}`, path), nil).Text
	if out != "error: offset 9 is beyond end of file (2 lines total)" {
		t.Fatalf("got: %q", out)
	}
}

func TestReadSuggestsTheNextOffset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	var lines strings.Builder
	for i := range 2000 {
		fmt.Fprintf(&lines, "line %d\n", i)
	}
	os.WriteFile(path, []byte(lines.String()), 0o644)

	out := ReadTool(NewLocal("")).Run(fmt.Sprintf(`{"path":"%s"}`, path), nil).Text
	if !strings.Contains(out, "[Showing lines 1-") || !strings.Contains(out, "limit). Use offset=") {
		t.Fatalf("sin aviso de truncado: %q", out[len(out)-200:])
	}
	next := out[strings.LastIndex(out, "Use offset=")+len("Use offset="):]
	next = next[:strings.Index(next, " ")]
	out = ReadTool(NewLocal("")).Run(fmt.Sprintf(`{"path":"%s","offset":%s}`, path, next), nil).Text
	if strings.Contains(out, "beyond end") {
		t.Fatalf("el offset sugerido no sirve: %q", out[:80])
	}
}

func TestBashTruncationNotice(t *testing.T) {
	bash := BashTool(NewLocal(""))
	out := bash.Run(`{"command":"yes | head -c 20000"}`, nil).Text
	if !strings.Contains(out, "Output truncated to the last 16KB") {
		t.Fatalf("got: %q", tail(out, 200))
	}
	if !strings.Contains(out, "Full output:") {
		t.Fatalf("got: %q", tail(out, 200))
	}
	path := strings.TrimSuffix(strings.SplitN(out, "Full output: ", 2)[1], "]")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("falta el archivo completo: %v", err)
	}
	os.Remove(path)
}

func TestBashFoldsExitStatusAndTimeout(t *testing.T) {
	bash := BashTool(NewLocal(""))
	out := bash.Run(`{"command":"echo hola; exit 3"}`, nil).Text
	if out != "hola\nerror: exit status 3" {
		t.Fatalf("got: %q", out)
	}
	out = bash.Run(`{"command":"sleep 5","timeout":1}`, nil).Text
	if !strings.HasSuffix(out, "error: command timed out after 1 seconds") {
		t.Fatalf("got: %q", out)
	}
	out = bash.Run(`{"command":"sleep 1","timeout":0}`, nil).Text
	if out != "error: invalid timeout: must be a positive number of seconds" {
		t.Fatalf("got: %q", out)
	}
}

func TestBashRunsInTheMachineDirectory(t *testing.T) {
	dir := t.TempDir()
	out := BashTool(NewLocal(dir)).Run(`{"command":"pwd"}`, nil).Text
	if !strings.Contains(out, dir) {
		t.Fatalf("corrio en otro lado: %q", out)
	}
}

func TestKillChildrenReapsSpawnedGroup(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if !registerPgid(cmd.Process.Pid) {
		t.Fatal("no se registro el grupo")
	}
	KillChildren()
	if err := cmd.Wait(); err == nil {
		t.Fatal("el hijo deberia haber muerto por senal")
	}
}

func TestReapGivesUpOnLiveChild(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	started := time.Now()
	reap(done, 200*time.Millisecond)
	if time.Since(started) > 2*time.Second {
		t.Fatal("reap no cedio")
	}
	syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	<-done
}

func TestToolsGoThroughTheMachineTheyAreGiven(t *testing.T) {
	fake := newFakeMachine()

	out := WriteTool(fake).Run(`{"path":"/memoria/nota.txt","content":"hola\n"}`, nil).Text
	if !strings.HasPrefix(out, "wrote /memoria/nota.txt") {
		t.Fatalf("got: %q", out)
	}

	out = ReadTool(fake).Run(`{"path":"/memoria/nota.txt"}`, nil).Text
	if strings.TrimSpace(out) != "hola" {
		t.Fatalf("got: %q", out)
	}

	out = EditTool(fake).Run(`{"path":"/memoria/nota.txt","edits":[{"oldText":"hola","newText":"chau"}]}`, nil).Text
	if !strings.Contains(out, "Successfully replaced 1 block(s)") {
		t.Fatalf("got: %q", out)
	}
	if string(fake.files["/memoria/nota.txt"]) != "chau\n" {
		t.Fatalf("guardado: %q", fake.files["/memoria/nota.txt"])
	}

	out = BashTool(fake).Run(`{"command":"ls /memoria"}`, nil).Text
	if out != "ran: ls /memoria" || strings.Join(fake.runs, ",") != "ls /memoria" {
		t.Fatalf("got: %q %v", out, fake.runs)
	}
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

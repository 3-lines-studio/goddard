package axe

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocalStatsAFileADirectoryAndSomethingMissing(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "dir"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hola"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	machine := NewLocal(root)

	file, err := machine.Stat("a.txt")
	if err != nil {
		t.Fatalf("stat del archivo: %v", err)
	}
	if file.Name != "a.txt" || file.IsDir || file.Size != 4 {
		t.Fatalf("el archivo quedó %+v", file)
	}

	dir, err := machine.Stat("dir")
	if err != nil {
		t.Fatalf("stat del directorio: %v", err)
	}
	if dir.Name != "dir" || !dir.IsDir {
		t.Fatalf("el directorio quedó %+v", dir)
	}

	if _, err := machine.Stat("no-existe.txt"); !os.IsNotExist(err) {
		t.Fatalf("lo que no está dio %v", err)
	}
}

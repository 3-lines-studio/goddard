package machine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUnTopeSinLimiteValeCero(t *testing.T) {
	if got := number("max\n"); got != 0 {
		t.Fatalf("un tope sin límite dio %d", got)
	}
	if got := number("1936084992\n"); got != 1936084992 {
		t.Fatalf("un tope con número dio %d", got)
	}
	if got := number(""); got != 0 {
		t.Fatalf("un archivo vacío dio %d", got)
	}
}

func TestElStatSeLeePorNombreYNoPorPosicion(t *testing.T) {
	stat := "anon 30408704\nfile 69615616\nkernel 275836928\nslab 106657912\n"
	if got := field(stat, "anon"); got != 30408704 {
		t.Fatalf("anon dio %d", got)
	}
	if got := field(stat, "file"); got != 69615616 {
		t.Fatalf("file dio %d", got)
	}
	if got := field(stat, "kernel"); got != 275836928 {
		t.Fatalf("kernel dio %d", got)
	}
	if got := field(stat, "sock"); got != 0 {
		t.Fatalf("una línea que no está dio %d", got)
	}
}

func TestLaMaquinaSeLeeDeVerdad(t *testing.T) {
	volume := t.TempDir()
	usage := Usage(volume)
	if usage.Memory.Used == 0 {
		t.Fatalf("la memoria del cgroup quedó en cero: %+v", usage)
	}
	if usage.Memory.Anon == 0 {
		t.Fatalf("la memoria anónima quedó en cero: %+v", usage)
	}
	if usage.Disk.Total == 0 {
		t.Fatalf("el volumen no tiene tamaño: %+v", usage)
	}
	if usage.Processes == 0 {
		t.Fatalf("no hay procesos: %+v", usage)
	}
}

func TestUnVolumenQueNoExisteNoRompe(t *testing.T) {
	usage := Usage(filepath.Join(t.TempDir(), "no-esta"))
	if usage.Disk.Total != 0 {
		t.Fatalf("un volumen que no existe dio %+v", usage.Disk)
	}
}

func TestElVolumenEsElQueSeLePasa(t *testing.T) {
	volume := t.TempDir()
	before := Usage(volume)
	write(t, filepath.Join(volume, "grande"), 4_000_000)
	after := Usage(volume)
	if after.Disk.Used <= before.Disk.Used {
		t.Fatalf("escribir no movió el volumen: %d y después %d", before.Disk.Used, after.Disk.Used)
	}
}

func write(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatalf("no pude escribir %s: %v", path, err)
	}
}

package app

import "testing"

func TestTheMessageCarriesTheFiles(t *testing.T) {
	got := messageOf("miralo", []attached{{Path: "files/c1/nota.txt", Mime: "text/plain"}})
	want := "miralo\n\nArchivos adjuntos:\n- files/c1/nota.txt (text/plain)\n"
	if got != want {
		t.Fatalf("el mensaje quedó %q", got)
	}
	if got := messageOf("solo texto", nil); got != "solo texto" {
		t.Fatalf("sin archivos quedó %q", got)
	}
	if got := messageOf("", []attached{{Path: "files/c1/foto.png", Mime: "image/png"}}); got != "Archivos adjuntos:\n- files/c1/foto.png (image/png)\n" {
		t.Fatalf("sin texto quedó %q", got)
	}
}

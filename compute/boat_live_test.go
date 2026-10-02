package compute

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// TestBoatLive is the whole way against the real API: it asks for a machine,
// leaves the key, gets in over SSH, writes and reads, stops, resumes, checks
// that the disk survived and puts a port behind a URL. It skips without a
// token, and it deletes what it created.
func TestBoatLive(t *testing.T) {
	token := os.Getenv("BOAT_API_KEY")
	if token == "" {
		t.Skip("sin BOAT_API_KEY no hay a quién pedirle una máquina")
	}
	boat := NewBoat(token)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Minute)
	defer cancel()

	started := time.Now()
	one, err := boat.Create(ctx, Spec{Size: "small", TTL: time.Hour})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Logf("create: %s, %s en %s", time.Since(started).Round(time.Millisecond), one.ID, one.State)
	defer func() {
		if err := boat.Delete(context.WithoutCancel(ctx), one.ID); err != nil {
			t.Logf("delete: %v", err)
		}
	}()

	ready := time.Now()
	one, err = boat.Wait(ctx, one.ID)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	t.Logf("lista en %s, en %s como %s", time.Since(ready).Round(time.Millisecond), one.Addr, one.User)

	public, private := newTestKey(t)
	authorized := time.Now()
	if err := boat.Authorize(ctx, one.ID, public); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	t.Logf("llave autorizada en %s", time.Since(authorized).Round(time.Millisecond))

	channel := NewSSH(one.Addr, one.User, private, nil)
	machine := NewMachine(channel, "/home/user/volumes/prueba")
	out := machine.Run("uname -srm; git --version; rg --version | head -1", 60, nil)
	t.Logf("la máquina contesta:\n%s", out)

	content := bytes.Repeat([]byte{0, 1, 2, 3, 4, 5, 6, 7}, 512*1024)
	wrote := time.Now()
	if err := machine.Write("grande.bin", content); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Logf("4 MB escritos en %s", time.Since(wrote).Round(time.Millisecond))
	read, err := machine.Read("grande.bin")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(read, content) {
		t.Fatalf("leyó %d bytes de %d", len(read), len(content))
	}

	stopped := time.Now()
	if err := boat.Stop(ctx, one.ID); err != nil {
		t.Fatalf("stop: %v", err)
	}
	t.Logf("parada en %s", time.Since(stopped).Round(time.Millisecond))
	if err := boat.Resume(ctx, one.ID); err != nil {
		t.Fatalf("resume: %v", err)
	}
	resumed := time.Now()
	if _, err := boat.Wait(ctx, one.ID); err != nil {
		t.Fatalf("wait después de resumir: %v", err)
	}
	t.Logf("de vuelta en %s", time.Since(resumed).Round(time.Millisecond))

	after, err := machine.Read("grande.bin")
	if err != nil {
		t.Fatalf("el archivo no sobrevivió: %v", err)
	}
	if !bytes.Equal(after, content) {
		t.Fatalf("después de resumir leyó %d bytes de %d", len(after), len(content))
	}
	t.Log("el archivo sobrevivió al stop y al resume")

	if err := machine.Write("sitio/index.html", []byte("<h1>hola</h1>")); err != nil {
		t.Fatalf("write del sitio: %v", err)
	}
	if out := machine.Run("nohup python3 -m http.server 8080 --directory /home/user/volumes/prueba/sitio > /tmp/sitio.log 2>&1 & sleep 2; echo listo", 60, nil); !strings.Contains(out, "listo") {
		t.Logf("el servidor dijo %q", out)
	}
	url, err := boat.Host(ctx, one.ID, 8080)
	if err != nil {
		t.Fatalf("host: %v", err)
	}
	t.Logf("el puerto quedó en %s", url)
	served := machine.Run("host 8080 --public", 60, nil)
	if !strings.Contains(served, "http") {
		t.Logf("el sandbox dijo %q", served)
	}
	curl := exec.CommandContext(ctx, "curl", "-sSL", "-m", "30", url)
	body, err := curl.Output()
	if err != nil {
		t.Fatalf("curl a %s: %v", url, err)
	}
	if !strings.Contains(string(body), "hola") {
		t.Fatalf("el sitio contestó %q", body)
	}
	t.Log("el sitio se ve desde afuera")
}

// newTestKey is a pair of the shape boat wants: the public half is left on the
// machine and the private one opens it.
func newTestKey(t *testing.T) (string, []byte) {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("no pude generar la llave: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatalf("no pude firmar: %v", err)
	}
	public := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))
	block, err := ssh.MarshalPrivateKey(private, "")
	if err != nil {
		t.Fatalf("no pude armar el PEM: %v", err)
	}
	return public + " prueba@goddard", pem.EncodeToMemory(block)
}

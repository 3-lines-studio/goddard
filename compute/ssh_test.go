package compute

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func newTestSSH(t *testing.T) *SSH {
	t.Helper()
	clientPublic, clientPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("llave del cliente: %v", err)
	}
	_, hostPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("llave del host: %v", err)
	}
	hostSigner, err := ssh.NewSignerFromKey(hostPrivate)
	if err != nil {
		t.Fatalf("firma del host: %v", err)
	}
	want, err := ssh.NewPublicKey(clientPublic)
	if err != nil {
		t.Fatalf("llave pública: %v", err)
	}
	config := &ssh.ServerConfig{
		PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if bytes.Equal(key.Marshal(), want.Marshal()) {
				return nil, nil
			}
			return nil, fmt.Errorf("esa llave no")
		},
	}
	config.AddHostKey(hostSigner)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go serveSSH(conn, config)
		}
	}()

	encoded, err := ssh.MarshalPrivateKey(clientPrivate, "")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return NewSSH(listener.Addr().String(), "tester", pem.EncodeToMemory(encoded))
}

func serveSSH(conn net.Conn, config *ssh.ServerConfig) {
	server, channels, requests, err := ssh.NewServerConn(conn, config)
	if err != nil {
		return
	}
	defer server.Close()
	go ssh.DiscardRequests(requests)
	for channel := range channels {
		if channel.ChannelType() != "session" {
			channel.Reject(ssh.UnknownChannelType, "solo session")
			continue
		}
		session, requests, err := channel.Accept()
		if err != nil {
			continue
		}
		go serveSession(session, requests)
	}
}

func serveSession(channel ssh.Channel, requests <-chan *ssh.Request) {
	for request := range requests {
		if request.Type != "exec" {
			request.Reply(false, nil)
			continue
		}
		var payload struct{ Command string }
		if err := ssh.Unmarshal(request.Payload, &payload); err != nil {
			request.Reply(false, nil)
			return
		}
		request.Reply(true, nil)
		command := exec.Command("bash", "-c", payload.Command)
		command.Stdin = channel
		command.Stdout = channel
		command.Stderr = channel.Stderr()
		status := 0
		if err := command.Run(); err != nil {
			var exited *exec.ExitError
			if errors.As(err, &exited) {
				status = exited.ExitCode()
			} else {
				status = 1
			}
		}
		channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{uint32(status)}))
		channel.Close()
		return
	}
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

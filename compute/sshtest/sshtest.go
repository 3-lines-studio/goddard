// Package sshtest is a machine of a test: an ssh server that runs what it is
// asked where the test runs, with a host key of its own, and that only lets in
// the client key it minted. It is what a test that reaches a sandbox the way
// production does — over ssh, with the three secrets of an owner — plugs in.
package sshtest

import (
	"bytes"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"testing"

	"golang.org/x/crypto/ssh"
)

// Server is where that machine is, who to be there and the key that opens it:
// the same secrets a sandbox is loaded with, and a key kept under a word when
// the passphrase is not empty.
type Server struct {
	Addr string
	User string
	Key  []byte
}

// New is a machine whose client key is kept in the open, and NewWithPassphrase
// one whose key is kept under a word, which is how a key ends up in a machine
// somebody else owns.
func New(t *testing.T) Server {
	t.Helper()
	return newServer(t, "")
}

func NewWithPassphrase(t *testing.T, passphrase string) Server {
	t.Helper()
	return newServer(t, passphrase)
}

func newServer(t *testing.T, passphrase string) Server {
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
			go serve(conn, config)
		}
	}()

	encoded, err := marshal(clientPrivate, passphrase)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return Server{Addr: listener.Addr().String(), User: "tester", Key: pem.EncodeToMemory(encoded)}
}

func marshal(key crypto.PrivateKey, passphrase string) (*pem.Block, error) {
	if passphrase == "" {
		return ssh.MarshalPrivateKey(key, "")
	}
	return ssh.MarshalPrivateKeyWithPassphrase(key, "", []byte(passphrase))
}

func serve(conn net.Conn, config *ssh.ServerConfig) {
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

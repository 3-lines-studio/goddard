package compute

import (
	"bytes"
	"context"
	"errors"
	"net"

	"golang.org/x/crypto/ssh"
)

type SSH struct {
	address    string
	user       string
	key        []byte
	passphrase []byte
}

// NewSSH is how a turn gets in: where the machine is, who to be there and the
// key that opens it. The passphrase is the word the key was kept with, and an
// empty one is a key that was not kept with any.
func NewSSH(address, user string, key, passphrase []byte) *SSH {
	return &SSH{address: address, user: user, key: key, passphrase: passphrase}
}

func (s *SSH) Exec(ctx context.Context, command string, stdin []byte) (Result, error) {
	client, err := s.dial(ctx)
	if err != nil {
		return Result{}, err
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return Result{}, err
	}
	defer session.Close()
	session.Stdin = bytes.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	done := make(chan error, 1)
	go func() { done <- session.Run(command) }()
	select {
	case err := <-done:
		result := Result{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
		if err == nil {
			return result, nil
		}
		var exited *ssh.ExitError
		if errors.As(err, &exited) {
			result.Exit = exited.ExitStatus()
			return result, nil
		}
		if ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
		return Result{}, err
	case <-ctx.Done():
		_ = session.Signal(ssh.SIGKILL)
		_ = session.Close()
		<-done
		return Result{}, ctx.Err()
	}
}

func (s *SSH) dial(ctx context.Context) (*ssh.Client, error) {
	signer, err := s.signer()
	if err != nil {
		return nil, err
	}
	config := &ssh.ClientConfig{
		User:            s.user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", s.address)
	if err != nil {
		return nil, err
	}
	connection, channels, requests, err := ssh.NewClientConn(conn, s.address, config)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return ssh.NewClient(connection, channels, requests), nil
}

// signer is the key that opens the machine. A key kept with a passphrase is
// the same key: what changes is that somebody has to say the word. A key that
// asks for one and is not told says so, instead of the machine refusing the
// key or answering that it could not get in.
func (s *SSH) signer() (ssh.Signer, error) {
	if len(s.passphrase) == 0 {
		signer, err := ssh.ParsePrivateKey(s.key)
		var locked *ssh.PassphraseMissingError
		if errors.As(err, &locked) {
			return nil, errors.New("la llave pide una passphrase y no está cargada")
		}
		return signer, err
	}
	return ssh.ParsePrivateKeyWithPassphrase(s.key, s.passphrase)
}

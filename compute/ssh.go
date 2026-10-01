package compute

import (
	"bytes"
	"context"
	"errors"
	"net"

	"golang.org/x/crypto/ssh"
)

type SSH struct {
	address string
	user    string
	key     []byte
}

func NewSSH(address, user string, key []byte) *SSH {
	return &SSH{address: address, user: user, key: key}
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
	signer, err := ssh.ParsePrivateKey(s.key)
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

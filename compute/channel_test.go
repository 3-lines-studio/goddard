package compute

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// noStdinChannel is a channel of the cloud: it runs a command and keeps its
// stdout, stderr and exit, but it has nothing to hand it on stdin. What the
// machine writes and reads has to go through Put and Get, which is why the
// interface has them.
type noStdinChannel struct {
	t *testing.T
}

func (c noStdinChannel) Exec(ctx context.Context, command string, stdin []byte) (Result, error) {
	if len(stdin) != 0 {
		c.t.Fatalf("le mandaron %d bytes por stdin a un canal que no lo tiene", len(stdin))
	}
	return Shell{}.Exec(ctx, command, nil)
}

func (c noStdinChannel) Put(ctx context.Context, path string, content []byte) error {
	return Shell{}.Put(ctx, path, content)
}

func (c noStdinChannel) Get(ctx context.Context, path string) ([]byte, error) {
	return Shell{}.Get(ctx, path)
}

func TestTheMachineWorksOnAChannelWithoutStdin(t *testing.T) {
	machine := NewMachine(noStdinChannel{t: t}, t.TempDir())
	if err := machine.Write("dir/nota.md", []byte("hola")); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := machine.Read("dir/nota.md")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "hola" {
		t.Fatalf("leyó %q", got)
	}
	if out := machine.Run("echo listo", 0, nil); !strings.Contains(out, "listo") {
		t.Fatalf("el comando devolvió %q", out)
	}
}

func TestTheMachineMovesAFileTooBigForACommandLine(t *testing.T) {
	machine := NewMachine(noStdinChannel{t: t}, t.TempDir())
	content := bytes.Repeat([]byte{0, 1, 2, 3, 4, 5, 6, 7}, 512*1024)
	if err := machine.Write("dir/grande.bin", content); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := machine.Read("dir/grande.bin")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("leyó %d bytes de %d", len(got), len(content))
	}
}

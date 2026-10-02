package compute

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"time"
)

// Shell is the channel of a sandbox that is this machine: the commands run
// where goddard runs. Production never uses it — a turn runs in the sandbox of
// its owner, which is another machine — and it is what anything that needs a
// real machine without a sandbox gets: the tests of the machine, and the tests
// of the app.
type Shell struct{}

func (Shell) Exec(ctx context.Context, command string, stdin []byte) (Result, error) {
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	ownGroup(cmd)
	cmd.WaitDelay = time.Second
	cmd.Stdin = bytes.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		return Result{}, ctx.Err()
	}
	result := Result{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if err == nil {
		return result, nil
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		return Result{}, err
	}
	result.Exit = exit.ExitCode()
	return result, nil
}

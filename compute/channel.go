package compute

import "context"

type Result struct {
	Stdout []byte
	Stderr []byte
	Exit   int
}

// A Channel is how a machine is reached: one command at a time, with what it
// printed and how it ended, and the two ways to move a file in and out. Put
// makes the directory of the file when it is not there, so the caller does not
// have to know how the other side makes one.
type Channel interface {
	Exec(ctx context.Context, command string, stdin []byte) (Result, error)
	Put(ctx context.Context, path string, content []byte) error
	Get(ctx context.Context, path string) ([]byte, error)
}

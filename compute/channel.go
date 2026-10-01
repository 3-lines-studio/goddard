package compute

import "context"

type Result struct {
	Stdout []byte
	Stderr []byte
	Exit   int
}

type Channel interface {
	Exec(ctx context.Context, command string, stdin []byte) (Result, error)
}

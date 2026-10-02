package compute

import (
	"context"
	"time"
)

// Spec is what the house asks a machine to be. What each field means is the
// provider's: a size is a name it knows, an image is one it can boot, and a
// setup script is what runs once the machine is up.
type Spec struct {
	Size  string
	Image string
	TTL   time.Duration
	Setup string
	Dir   string
}

// Sandbox is a machine of the house as the provider sees it: an id to ask about
// it later, what it is doing right now, and where to reach it over SSH.
type Sandbox struct {
	ID    string
	State string
	Addr  string
	User  string
}

// A Provider is who makes a machine when the owner has none of their own: the
// house. It is not how a turn runs — that is the Channel — but how a machine
// comes to exist, goes to sleep and goes away. A provider that gives SSH leaves
// the turn to compute.SSH; the address and the user are all it has to hand over.
type Provider interface {
	Create(ctx context.Context, spec Spec) (Sandbox, error)
	Get(ctx context.Context, id string) (Sandbox, error)
	Stop(ctx context.Context, id string) error
	Resume(ctx context.Context, id string) error
	Delete(ctx context.Context, id string) error
	Authorize(ctx context.Context, id string, publicKey string) error
	Host(ctx context.Context, id string, port int) (string, error)
}

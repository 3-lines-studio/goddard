package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/3-lines-studio/goddard/chat"
	"github.com/3-lines-studio/goddard/heimdall"
)

// The sandbox of an owner is the machine that runs the tools of a turn, and it
// is reached with three secrets of that owner in heimdall, under a project of
// its own so they are not mixed with the secrets of the projects: where it is,
// who to be there and the key to get in. None of the three is written down
// anywhere else, so a sandbox moves without touching the workspace.
const (
	SandboxProject = "sandbox"
	SandboxEnv     = "default"

	SandboxAddr = "SSH_ADDR"
	SandboxUser = "SSH_USER"
	SandboxKey  = "SSH_KEY"
)

// Sandbox is how a turn gets into the machine of an owner.
type Sandbox struct {
	Addr string
	User string
	Key  []byte
}

// ErrNoSandbox is what a turn gets when its owner loaded no sandbox: without
// one the tools have nowhere to run, and there is no local fallback.
var ErrNoSandbox = errors.New("ese workspace no tiene sandbox")

// Sandbox reads the three secrets of the sandbox of an owner. It is what a
// turn asks for before running anything.
func (s *Service) Sandbox(ctx context.Context, owner chat.Owner) (Sandbox, error) {
	secrets, err := s.Heimdall.Secrets(ctx, heimdallOwner(owner), SandboxProject, SandboxEnv)
	if err != nil {
		return Sandbox{}, err
	}
	found := Sandbox{Addr: secrets[SandboxAddr], User: secrets[SandboxUser], Key: []byte(secrets[SandboxKey])}
	if missing := found.missing(); missing != "" {
		return Sandbox{}, fmt.Errorf("%w: le falta %s", ErrNoSandbox, missing)
	}
	return found, nil
}

// missing is the first of the three that is not there, so whoever is loading a
// sandbox is told which one and not only that something is missing.
func (s Sandbox) missing() string {
	switch {
	case s.Addr == "":
		return SandboxAddr
	case s.User == "":
		return SandboxUser
	case len(s.Key) == 0:
		return SandboxKey
	}
	return ""
}

// heimdallOwner is the same owner in the terms of the vault: the sandbox of a
// workspace is the sandbox of whoever owns it.
func heimdallOwner(owner chat.Owner) heimdall.Owner {
	if owner.Kind == chat.OwnerOrg {
		return heimdall.Org(owner.ID)
	}
	return heimdall.User(owner.ID)
}

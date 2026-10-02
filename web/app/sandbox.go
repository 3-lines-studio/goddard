package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/3-lines-studio/goddard/chat"
	"github.com/3-lines-studio/goddard/compute"
	"github.com/3-lines-studio/goddard/heimdall"
)

// The sandbox of an owner is the machine that runs the tools of a turn, and it
// is reached with the secrets of that owner in heimdall, under a project of
// its own so they are not mixed with the secrets of the projects: where it is,
// who to be there, the key to get in and the word the key was kept with, when
// it has one. None of them is written down anywhere else, so a sandbox moves
// without touching the workspace.
const (
	SandboxProject = "sandbox"
	SandboxEnv     = "default"

	SandboxAddr       = "SSH_ADDR"
	SandboxUser       = "SSH_USER"
	SandboxKey        = "SSH_KEY"
	SandboxPassphrase = "SSH_PASSPHRASE"
)

// Sandbox is how a turn gets into the machine of an owner, as it is loaded:
// what is there, complete or not, which is what the panel that loads it shows.
// `Ready` is what says whether a turn can use it. The passphrase is the word
// the key was kept with, and most keys have none.
type Sandbox struct {
	Addr       string
	User       string
	Key        []byte
	Passphrase []byte
}

// ErrNoSandbox is what a turn gets when its owner loaded no sandbox: without
// one the tools have nowhere to run, and there is no local fallback.
var ErrNoSandbox = errors.New("ese workspace no tiene sandbox")

// Dialer is how a turn reaches the sandbox of an owner: the machine that runs
// its tools. There is only one way in — a turn never runs in the container
// goddard runs in, which carries no tools — and a test that needs a machine
// without a sandbox plugs a channel of its own here.
type Dialer interface {
	Dial(sandbox Sandbox) compute.Channel
}

// SSH is the way in: the secrets of the sandbox, over ssh.
type SSH struct{}

func (SSH) Dial(sandbox Sandbox) compute.Channel {
	return compute.NewSSH(sandbox.Addr, sandbox.User, sandbox.Key, sandbox.Passphrase)
}

// Sandbox reads the secrets of the sandbox of an owner.
func (s *Service) Sandbox(ctx context.Context, owner chat.Owner) (Sandbox, error) {
	secrets, err := s.Heimdall.Secrets(ctx, heimdallOwner(owner), SandboxProject, SandboxEnv)
	if err != nil {
		return Sandbox{}, err
	}
	return Sandbox{
		Addr:       secrets[SandboxAddr],
		User:       secrets[SandboxUser],
		Key:        []byte(secrets[SandboxKey]),
		Passphrase: []byte(secrets[SandboxPassphrase]),
	}, nil
}

// Ready says whether a sandbox has the three pieces a turn needs, and which
// one is missing when it does not: whoever loads a sandbox is told which one,
// and not only that something is missing.
func (sandbox Sandbox) Ready() error {
	missing := ""
	switch {
	case sandbox.Addr == "":
		missing = SandboxAddr
	case sandbox.User == "":
		missing = SandboxUser
	case len(sandbox.Key) == 0:
		missing = SandboxKey
	}
	if missing != "" {
		return fmt.Errorf("%w: le falta %s", ErrNoSandbox, missing)
	}
	return nil
}

// heimdallOwner is the same owner in the terms of the vault: the sandbox of a
// workspace is the sandbox of whoever owns it.
func heimdallOwner(owner chat.Owner) heimdall.Owner {
	if owner.Kind == chat.OwnerOrg {
		return heimdall.Org(owner.ID)
	}
	return heimdall.User(owner.ID)
}

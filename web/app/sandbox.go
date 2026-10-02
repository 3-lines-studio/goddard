package app

import (
	"context"
	"errors"

	"github.com/3-lines-studio/goddard/chat"
	"github.com/3-lines-studio/goddard/heimdall"
)

// The sandbox of an owner is the machine that runs the tools of a turn: where
// it is and who to be there are the workspace of that owner, and the key that
// opens it is a secret of the same owner in heimdall, under a project of its
// own so it is not mixed with the secrets of the projects.
const (
	SandboxProject = "sandbox"
	SandboxEnv     = "default"
	SandboxKeyName = "SSH_KEY"
)

// ErrNoSandboxKey is what a turn gets when its owner loaded no key: without a
// sandbox the tools have nowhere to run, and there is no local fallback.
var ErrNoSandboxKey = errors.New("ese workspace no tiene la llave del sandbox")

// SandboxKey is the private key of the sandbox of an owner, read from
// heimdall. The key never lands anywhere else: the workspace keeps where the
// machine is, and this is the only part of reaching it that is a secret.
func (s *Service) SandboxKey(ctx context.Context, owner chat.Owner) ([]byte, error) {
	secrets, err := s.Heimdall.Secrets(ctx, heimdallOwner(owner), SandboxProject, SandboxEnv)
	if err != nil {
		return nil, err
	}
	key := secrets[SandboxKeyName]
	if key == "" {
		return nil, ErrNoSandboxKey
	}
	return []byte(key), nil
}

// heimdallOwner is the same owner in the terms of the vault: the sandbox of a
// workspace is the sandbox of whoever owns it.
func heimdallOwner(owner chat.Owner) heimdall.Owner {
	if owner.Kind == chat.OwnerOrg {
		return heimdall.Org(owner.ID)
	}
	return heimdall.User(owner.ID)
}

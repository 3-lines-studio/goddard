package app

import (
	"context"
	"time"

	"github.com/3-lines-studio/goddard/chat"
	"github.com/3-lines-studio/goddard/workspace"
)

// checkTimeout is how long the check of a sandbox waits: it is a button, and a
// machine that does not answer is an answer.
const checkTimeout = 15 * time.Second

// Workspace is the workspace of an owner: the row, or the volume of that owner
// when there is none, which is where its projects live by default.
func (s *Service) Workspace(ctx context.Context, owner chat.Owner) (workspace.Workspace, error) {
	one := workspaceOwner(owner)
	space, ok, err := s.Workspaces.Get(ctx, one)
	if err != nil {
		return workspace.Workspace{}, err
	}
	if !ok {
		return workspace.Workspace{Owner: one, Path: workspace.DefaultPath(s.Volumes, one)}, nil
	}
	return space, nil
}

// SaveWorkspace writes where the projects of an owner live and how to reach the
// machine that runs them. What comes empty is left as it was: the key is never
// read back, so the form that loads a sandbox cannot send it again and an empty
// box means "the one that is already there".
func (s *Service) SaveWorkspace(ctx context.Context, owner chat.Owner, path string, sandbox Sandbox, actor string) error {
	current, err := s.Workspace(ctx, owner)
	if err != nil {
		return err
	}
	row := workspace.Workspace{Owner: workspaceOwner(owner), Path: path, Provider: current.Provider, SandboxID: current.SandboxID}
	if err := s.Workspaces.Set(ctx, row, actor); err != nil {
		return err
	}
	for _, one := range []struct{ name, value string }{
		{SandboxAddr, sandbox.Addr},
		{SandboxUser, sandbox.User},
		{SandboxKey, string(sandbox.Key)},
		{SandboxPassphrase, string(sandbox.Passphrase)},
	} {
		if one.value == "" {
			continue
		}
		if err := s.Heimdall.Set(ctx, heimdallOwner(owner), SandboxProject, SandboxEnv, one.name, one.value, actor); err != nil {
			return err
		}
	}
	return nil
}

// workspaceOwner is the same owner in the terms of the workspace: the volume
// and the projects inside it are the owner's, and heimdall keeps that owner
// too.
func workspaceOwner(owner chat.Owner) workspace.Owner {
	return workspace.Owner{Kind: owner.Kind, ID: owner.ID}
}

// CheckSandbox dials a machine the way a turn would and says whether a turn
// could run in it: the machine answers, and the volume of that owner is mounted
// inside it. The secrets come from wherever the caller got them — loaded in
// heimdall, or just typed in a form — and nothing is written down here: a
// machine that does not answer is a machine that does not get loaded. It leaves
// nothing behind, not even the directory of a project.
func (s *Service) CheckSandbox(ctx context.Context, owner chat.Owner, volume string, sandbox Sandbox) error {
	if err := sandbox.Ready(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	return s.reach(ctx, owner, volume, s.Dialer.Dial(sandbox))
}

// RemoveWorkspace takes the workspace of an owner back to nothing: its projects
// go back to the volume of that owner and its sandbox stops being loaded, so a
// turn does not run there. It is what "esta máquina ya no" means, and it is
// idempotent: doing it twice leaves the same nothing.
func (s *Service) RemoveWorkspace(ctx context.Context, owner chat.Owner, actor string) error {
	secrets, err := s.Heimdall.Secrets(ctx, heimdallOwner(owner), SandboxProject, SandboxEnv)
	if err != nil {
		return err
	}
	for _, name := range []string{SandboxAddr, SandboxUser, SandboxKey, SandboxPassphrase} {
		if _, loaded := secrets[name]; !loaded {
			continue
		}
		if err := s.Heimdall.Unset(ctx, heimdallOwner(owner), SandboxProject, SandboxEnv, name, actor); err != nil {
			return err
		}
	}
	return s.Workspaces.Delete(ctx, workspaceOwner(owner))
}

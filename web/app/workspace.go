package app

import (
	"context"
	"fmt"
	"time"

	"github.com/3-lines-studio/goddard/chat"
	"github.com/3-lines-studio/goddard/compute"
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
	if err := s.Workspaces.Set(ctx, workspace.Workspace{Owner: workspaceOwner(owner), Path: path}, actor); err != nil {
		return err
	}
	for _, one := range []struct{ name, value string }{
		{SandboxAddr, sandbox.Addr},
		{SandboxUser, sandbox.User},
		{SandboxKey, string(sandbox.Key)},
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

// CheckWorkspace dials the sandbox of an owner the way a turn would and says
// whether it can run there: the machine answers and the volume of that owner is
// mounted inside it. It leaves nothing behind, not even the directory of a
// project, because the point is telling a mounted volume from a directory that
// looks like one.
func (s *Service) CheckWorkspace(ctx context.Context, owner chat.Owner) error {
	space, err := s.Workspace(ctx, owner)
	if err != nil {
		return err
	}
	sandbox, err := s.Sandbox(ctx, owner)
	if err != nil {
		return err
	}
	if err := sandbox.Ready(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	machine := compute.NewMachine(s.Dialer.Dial(sandbox), space.Path)
	mounted, err := machine.Mounted(ctx)
	if err != nil {
		return fmt.Errorf("no pude entrar a la máquina: %w", err)
	}
	if !mounted {
		return fmt.Errorf("la máquina contesta, pero no ve %s adentro", space.Path)
	}
	return nil
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
	for _, name := range []string{SandboxAddr, SandboxUser, SandboxKey} {
		if _, loaded := secrets[name]; !loaded {
			continue
		}
		if err := s.Heimdall.Unset(ctx, heimdallOwner(owner), SandboxProject, SandboxEnv, name, actor); err != nil {
			return err
		}
	}
	return s.Workspaces.Delete(ctx, workspaceOwner(owner))
}

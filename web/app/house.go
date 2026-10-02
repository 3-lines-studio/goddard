package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/3-lines-studio/goddard/chat"
	"github.com/3-lines-studio/goddard/compute"
	"github.com/3-lines-studio/goddard/workspace"
)

// The house is where a turn runs when its owner brought no machine: goddard asks
// a provider for one, keeps it for that owner and writes where it is in heimdall
// like any other sandbox, so nothing else in goddard — the turn, the panel, the
// volume check — tells a machine of the house from one the owner brought.
const (
	HouseProvider = "boat"
	HouseSize     = "small"
	HouseTTL      = 2 * time.Hour
	// HouseRoot is where the projects of an owner live inside a machine of the
	// house: it is the working directory of the sandbox user, and the disk of
	// the machine is what makes it last.
	HouseRoot = "/home/user/volumes"
)

// ErrNoHouse is what a turn gets when the owner has no sandbox and there is no
// house to ask one from.
var ErrNoHouse = errors.New("no hay máquina de la casa")

// houseSandbox is the machine of the house for an owner: the one goddard made
// for them before — woken up if it is asleep — or a new one, left ready to be
// used like any other sandbox.
func (s *Service) houseSandbox(ctx context.Context, owner chat.Owner) (Sandbox, error) {
	if s.House == nil {
		return Sandbox{}, ErrNoHouse
	}
	space, err := s.Workspace(ctx, owner)
	if err != nil {
		return Sandbox{}, err
	}
	if space.Provider != HouseProvider || space.SandboxID == "" {
		return s.makeHouse(ctx, owner, space)
	}
	return s.wakeHouse(ctx, owner, space)
}

// makeHouse asks for a machine, leaves the key of this owner on it and writes
// down where it is: the id in the row of the workspace and the way in among the
// secrets, which is where every turn reads it from.
func (s *Service) makeHouse(ctx context.Context, owner chat.Owner, space workspace.Workspace) (Sandbox, error) {
	public, private, err := compute.NewKeyPair()
	if err != nil {
		return Sandbox{}, err
	}
	asked, err := s.House.Create(ctx, compute.Spec{Size: HouseSize, TTL: HouseTTL, Dir: space.Path})
	if err != nil {
		return Sandbox{}, err
	}
	kept := false
	defer func() {
		if kept {
			return
		}
		if err := s.House.Delete(context.WithoutCancel(ctx), asked.ID); err != nil {
			log.Printf("goddard: dejé un sandbox de la casa sin dueño (%s): %v", asked.ID, err)
		}
	}()
	if err := s.House.Authorize(ctx, asked.ID, public); err != nil {
		return Sandbox{}, err
	}
	one, err := s.House.Wait(ctx, asked.ID)
	if err != nil {
		return Sandbox{}, err
	}
	sandbox := Sandbox{Addr: one.Addr, User: one.User, Key: private}
	space.Provider = HouseProvider
	space.SandboxID = asked.ID
	space.Path = filepath.Join(HouseRoot, owner.ID)
	if err := s.openDir(ctx, sandbox, space.Path); err != nil {
		return Sandbox{}, err
	}
	if err := s.writeSandbox(ctx, owner, space, sandbox, owner.ID); err != nil {
		return Sandbox{}, err
	}
	kept = true
	return sandbox, nil
}

// openDir makes the directory where the projects of an owner live, inside a
// machine that has just been made: a turn refuses to run without it, because a
// machine whose volume was never there writes where nothing lasts. It goes
// through the same channel a turn uses, which is the only thing that knows how
// to reach it.
func (s *Service) openDir(ctx context.Context, sandbox Sandbox, dir string) error {
	machine := compute.NewMachine(s.Dialer.Dial(sandbox), dir)
	out := machine.Run("true", 60, nil)
	if strings.Contains(out, "error:") {
		return fmt.Errorf("no pude armar %s en la máquina de la casa: %s", dir, strings.TrimSpace(out))
	}
	return nil
}

// wakeHouse is a machine of the house that is already there: a sandbox that is
// archived is resumed, and where it says it is now is written again, because a
// machine that sleeps and comes back does not always come back at the same
// address. The key of the owner is the same one as always.
func (s *Service) wakeHouse(ctx context.Context, owner chat.Owner, space workspace.Workspace) (Sandbox, error) {
	one, err := s.House.Get(ctx, space.SandboxID)
	if err != nil {
		return Sandbox{}, err
	}
	if one.State == "archived" || one.State == "stopped" || one.Addr == "" {
		if err := s.House.Resume(ctx, space.SandboxID); err != nil {
			return Sandbox{}, err
		}
		one, err = s.House.Wait(ctx, space.SandboxID)
		if err != nil {
			return Sandbox{}, err
		}
	}
	current, err := s.Sandbox(ctx, owner)
	if err != nil {
		return Sandbox{}, err
	}
	sandbox := Sandbox{Addr: one.Addr, User: one.User, Key: current.Key, Passphrase: current.Passphrase}
	if err := s.writeSandbox(ctx, owner, space, sandbox, owner.ID); err != nil {
		return Sandbox{}, err
	}
	return sandbox, nil
}

// StopHouse puts the machine of the house to sleep: it is what stops paying for
// it while nobody uses it. A sandbox that is not of the house is left alone —
// only its owner knows how to turn it off.
func (s *Service) StopHouse(ctx context.Context, owner chat.Owner) error {
	if s.House == nil {
		return ErrNoHouse
	}
	space, err := s.Workspace(ctx, owner)
	if err != nil {
		return err
	}
	if space.Provider != HouseProvider || space.SandboxID == "" {
		return nil
	}
	return s.House.Stop(ctx, space.SandboxID)
}

// writeSandbox leaves the machine of an owner written down: the id and the
// provider in the row of the workspace, and the way in among the secrets of the
// owner in heimdall, which is where the turn reads it.
func (s *Service) writeSandbox(ctx context.Context, owner chat.Owner, space workspace.Workspace, sandbox Sandbox, actor string) error {
	row := workspace.Workspace{
		Owner:     workspaceOwner(owner),
		Path:      space.Path,
		Provider:  space.Provider,
		SandboxID: space.SandboxID,
	}
	if err := s.Workspaces.Set(ctx, row, actor); err != nil {
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

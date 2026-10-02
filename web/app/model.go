package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/3-lines-studio/goddard/axe"
	"github.com/3-lines-studio/goddard/chat"
)

// The model of an owner is either the one of the house — what goddard answers
// with when nobody says otherwise, read from the environment — or the one the
// owner brought: where it answers, which model and the key to ask for it. The
// three are secrets of the owner in heimdall, under a project of their own so
// they are not mixed with the secrets of the projects, and that is the only
// place they live, the same way as the sandbox.
const (
	ModelProject = "model"
	ModelEnv     = "default"

	ModelBase = "BASE"
	ModelName = "MODEL"
	ModelKey  = "API_KEY"
)

// Model is the model an owner brought, as it is loaded: complete or not, which
// is what the panel that loads it shows. The key never comes back out of the
// store: `Ready` is what says whether a turn can use it.
type Model struct {
	Base string
	Name string
	Key  []byte
}

// ErrNoModel is what a turn gets when the model of an owner is half loaded:
// some of the three pieces are there and one is missing, and answering with the
// house because of that would be a silent lie.
var ErrNoModel = errors.New("el modelo propio de ese workspace está incompleto")

// Loaded says whether the owner brought a model at all: with none of the three
// loaded, a turn answers with the model of the house.
func (m Model) Loaded() bool {
	return m.Base != "" || m.Name != "" || len(m.Key) > 0
}

// Ready says whether that model can answer, and which piece is missing when it
// cannot.
func (m Model) Ready() error {
	missing := ""
	switch {
	case m.Base == "":
		missing = ModelBase
	case m.Name == "":
		missing = ModelName
	case len(m.Key) == 0:
		missing = ModelKey
	}
	if missing != "" {
		return fmt.Errorf("%w: le falta %s", ErrNoModel, missing)
	}
	return nil
}

// OwnModel reads the model an owner brought.
func (s *Service) OwnModel(ctx context.Context, owner chat.Owner) (Model, error) {
	secrets, err := s.Heimdall.Secrets(ctx, heimdallOwner(owner), ModelProject, ModelEnv)
	if err != nil {
		return Model{}, err
	}
	return Model{
		Base: secrets[ModelBase],
		Name: secrets[ModelName],
		Key:  []byte(secrets[ModelKey]),
	}, nil
}

// ModelOf is the model a turn of that owner answers with: the one it brought,
// or the provider and the name goddard was built with when it brought none.
func (s *Service) ModelOf(ctx context.Context, owner chat.Owner) (axe.Provider, string, error) {
	own, err := s.OwnModel(ctx, owner)
	if err != nil {
		return nil, "", err
	}
	if !own.Loaded() {
		return s.Provider, s.Model, nil
	}
	if err := own.Ready(); err != nil {
		return nil, "", err
	}
	return axe.NewOpenAI(own.Base, string(own.Key)), own.Name, nil
}

// SaveModel writes the model an owner brought. What comes empty is left as it
// was: the key is never read back, so the form that loads it cannot send it
// again and an empty box means "the one that is already there".
func (s *Service) SaveModel(ctx context.Context, owner chat.Owner, model Model, actor string) error {
	for _, one := range []struct{ name, value string }{
		{ModelBase, model.Base},
		{ModelName, model.Name},
		{ModelKey, string(model.Key)},
	} {
		if one.value == "" {
			continue
		}
		if err := s.Heimdall.Set(ctx, heimdallOwner(owner), ModelProject, ModelEnv, one.name, one.value, actor); err != nil {
			return err
		}
	}
	return nil
}

// ClearModel takes the model of an owner back to the one of the house: the
// three secrets stop being loaded so a turn answers with the house again. It is
// idempotent: doing it twice leaves the same nothing.
func (s *Service) ClearModel(ctx context.Context, owner chat.Owner, actor string) error {
	secrets, err := s.Heimdall.Secrets(ctx, heimdallOwner(owner), ModelProject, ModelEnv)
	if err != nil {
		return err
	}
	for _, name := range []string{ModelBase, ModelName, ModelKey} {
		if _, loaded := secrets[name]; !loaded {
			continue
		}
		if err := s.Heimdall.Unset(ctx, heimdallOwner(owner), ModelProject, ModelEnv, name, actor); err != nil {
			return err
		}
	}
	return nil
}

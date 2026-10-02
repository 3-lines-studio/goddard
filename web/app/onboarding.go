package app

import (
	"context"

	"github.com/3-lines-studio/goddard/chat"
)

// Onboarding is how much of the first setup of an owner is done: the model it
// answers with and the machine its tools run in. Nothing is written down to
// remember it — it is read from what the owner loaded, which is exactly what a
// turn asks for before it runs — so somebody coming back and somebody that
// never left see the same.
type Onboarding struct {
	Model   Model
	Sandbox Sandbox
}

// Done says whether goddard can already work for that owner: without a machine
// the tools have nowhere to run, and half a model is not a model. The name is
// not part of it: what somebody is called makes goddard nicer and not possible.
func (o Onboarding) Done() bool {
	if o.Sandbox.Ready() != nil {
		return false
	}
	return !o.Model.Loaded() || o.Model.Ready() == nil
}

// Onboarding reads it: the model and the sandbox of an owner as they are
// loaded, which is what a turn reads before it starts.
func (s *Service) Onboarding(ctx context.Context, owner chat.Owner) (Onboarding, error) {
	model, err := s.OwnModel(ctx, owner)
	if err != nil {
		return Onboarding{}, err
	}
	sandbox, err := s.Sandbox(ctx, owner)
	if err != nil {
		return Onboarding{}, err
	}
	return Onboarding{Model: model, Sandbox: sandbox}, nil
}

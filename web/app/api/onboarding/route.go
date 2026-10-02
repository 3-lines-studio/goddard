// Package onboarding is the first setup of a person: the model goddard answers
// with and the machine its tools run in. It is read and not remembered — the
// answer is the same state a turn reads before it runs — and the routes that
// load each half are the ones the panels already use.
package onboarding

import (
	"encoding/json"
	"net/http"

	"github.com/3-lines-studio/goddard/chat"
	space "github.com/3-lines-studio/goddard/web/app/api/workspace"
)

// view is the first setup as the page shows it. The keys never come back:
// what comes is whether there is one.
type view struct {
	Done    bool        `json:"done"`
	User    userView    `json:"user"`
	Model   modelView   `json:"model"`
	Compute computeView `json:"compute"`
}

type userView struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type modelView struct {
	Own    bool   `json:"own"`
	Base   string `json:"base"`
	Name   string `json:"name"`
	HasKey bool   `json:"has_key"`
	House  string `json:"house"`
}

type computeView struct {
	Path   string `json:"path"`
	Addr   string `json:"addr"`
	User   string `json:"user"`
	HasKey bool   `json:"has_key"`
}

// Get is how much of the first setup of whoever is asking is done, and what is
// loaded so far, which is what the page paints.
func Get(w http.ResponseWriter, r *http.Request) {
	service, user, ok := space.Session(w, r)
	if !ok {
		return
	}
	owner := chat.Owner{Kind: chat.OwnerUser, ID: user.ID}
	setup, err := service.Onboarding(r.Context(), owner)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	volumes, err := service.Workspace(r.Context(), owner)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	body := view{
		Done: setup.Done(),
		User: userView{Name: user.Name, Email: user.Email},
		Model: modelView{
			Own:    setup.Model.Loaded(),
			Base:   setup.Model.Base,
			Name:   setup.Model.Name,
			HasKey: len(setup.Model.Key) > 0,
			House:  service.Model,
		},
		Compute: computeView{
			Path:   volumes.Path,
			Addr:   setup.Sandbox.Addr,
			User:   setup.Sandbox.User,
			HasKey: len(setup.Sandbox.Key) > 0,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

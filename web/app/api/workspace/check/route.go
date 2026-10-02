// Package check is the button that says whether a machine answers: it dials it
// the way a turn would and looks for the volume of that owner inside it.
package check

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/3-lines-studio/goddard/chat"
	"github.com/3-lines-studio/goddard/web/app"
	space "github.com/3-lines-studio/goddard/web/app/api/workspace"
)

type request struct {
	Org        string `json:"org"`
	Path       string `json:"path"`
	Addr       string `json:"addr"`
	User       string `json:"user"`
	Key        string `json:"key"`
	Passphrase string `json:"passphrase"`
}

// answer is what the panel shows: whether a turn could run, and what went
// wrong when it could not.
type answer struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// Post checks a machine: the one an owner has loaded, or the one a form is
// about to load. It is a diagnostic and not a failure of the API: the 200 comes
// either way and what went wrong travels in the message, because a machine that
// does not answer is an answer too.
func Post(w http.ResponseWriter, r *http.Request) {
	service, user, ok := space.Session(w, r)
	if !ok {
		return
	}
	var body request
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "no pude leer la computadora", http.StatusBadRequest)
		return
	}
	owner, ok := space.OwnerOf(w, r, service, user, body.Org, false)
	if !ok {
		return
	}
	found := answer{OK: true}
	if err := dial(r.Context(), service, owner, body); err != nil {
		found.OK = false
		found.Message = err.Error()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(found)
}

// dial is the machine the check is about: what the form brought, with what is
// loaded completing what did not come, so a form that only changes the address
// does not send the key again. Nothing of it is written down: the check is a
// question, and a machine that does not answer is a machine that does not get
// loaded.
func dial(ctx context.Context, service *app.Service, owner chat.Owner, body request) error {
	volumes, err := service.Workspace(ctx, owner)
	if err != nil {
		return err
	}
	sandbox, err := service.Sandbox(ctx, owner)
	if err != nil {
		return err
	}
	if body.Path != "" {
		volumes.Path = body.Path
	}
	if body.Addr != "" {
		sandbox.Addr = body.Addr
	}
	if body.User != "" {
		sandbox.User = body.User
	}
	if body.Key != "" {
		sandbox.Key = []byte(body.Key)
	}
	if body.Passphrase != "" {
		sandbox.Passphrase = []byte(body.Passphrase)
	}
	return service.CheckSandbox(ctx, owner, volumes.Path, sandbox)
}

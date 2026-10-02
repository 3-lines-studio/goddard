// Package check is the button that says whether the sandbox of an owner
// answers: it dials the machine the way a turn would and looks for the volume
// of that owner inside it.
package check

import (
	"encoding/json"
	"net/http"

	space "github.com/3-lines-studio/goddard/web/app/api/workspace"
)

type request struct {
	Org string `json:"org"`
}

// answer is what the panel shows: whether a turn could run, and what went
// wrong when it could not.
type answer struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// Post checks the sandbox of an owner. It is a diagnostic and not a failure of
// the API: the 200 comes either way and what went wrong travels in the message,
// because a machine that does not answer is an answer too.
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
	if err := service.CheckWorkspace(r.Context(), owner); err != nil {
		found.OK = false
		found.Message = err.Error()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(found)
}

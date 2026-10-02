// Package me is what goddard calls somebody. The name that comes with the
// mail is a guess — the local part of it — and this is where it stops being
// one: it is what the prompt says and what the app shows.
package me

import (
	"encoding/json"
	"net/http"
	"strings"
	"unicode/utf8"

	space "github.com/3-lines-studio/goddard/web/app/api/workspace"
)

// maxName is how long a name may be: it travels in a prompt and in the rail,
// and neither wants an essay.
const maxName = 64

type request struct {
	Name string `json:"name"`
}

// Patch changes the name of whoever is asking. An empty name is nothing to
// call somebody, so it is refused instead of stored.
func Patch(w http.ResponseWriter, r *http.Request) {
	service, user, ok := space.Session(w, r)
	if !ok {
		return
	}
	var body request
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "no pude leer el nombre", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		http.Error(w, "el nombre está vacío", http.StatusBadRequest)
		return
	}
	if utf8.RuneCountInString(name) > maxName {
		http.Error(w, "el nombre es muy largo", http.StatusBadRequest)
		return
	}
	if err := service.Auth.SetName(r.Context(), user.ID, name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Package model is the model of an owner: the one it brought, or the one of the
// house. The panel reads which one is in use and loads or unloads the one of
// the owner; the key goes in and never comes back out.
package model

import (
	"encoding/json"
	"net/http"

	"github.com/3-lines-studio/goddard/chat"
	"github.com/3-lines-studio/goddard/web/app"
	space "github.com/3-lines-studio/goddard/web/app/api/workspace"
)

type request struct {
	Org  string `json:"org"`
	Base string `json:"base"`
	Name string `json:"name"`
	Key  string `json:"key"`
}

// view is the model of an owner as the panel shows it: what it brought, and the
// name of the one of the house, which is what answers when it brought nothing.
// The key never comes back: what comes is whether there is one.
type view struct {
	Owner  chat.Owner `json:"owner"`
	Own    bool       `json:"own"`
	Base   string     `json:"base"`
	Name   string     `json:"name"`
	HasKey bool       `json:"has_key"`
	House  string     `json:"house"`
}

// Get is the model of whoever is asking, or of an organization they are in.
func Get(w http.ResponseWriter, r *http.Request) {
	service, user, ok := space.Session(w, r)
	if !ok {
		return
	}
	owner, ok := space.OwnerOf(w, r, service, user, r.URL.Query().Get("org"), false)
	if !ok {
		return
	}
	body, err := current(r, service, owner)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

// Post loads the model of an owner: where it answers, which model and the key.
// What comes empty is left as it was, and the model of an organization is for
// its owners and its admins: that key is the one the team answers with.
func Post(w http.ResponseWriter, r *http.Request) {
	service, user, ok := space.Session(w, r)
	if !ok {
		return
	}
	var body request
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "no pude leer el modelo", http.StatusBadRequest)
		return
	}
	owner, ok := space.OwnerOf(w, r, service, user, body.Org, true)
	if !ok {
		return
	}
	err := service.SaveModel(r.Context(), owner, app.Model{
		Base: body.Base,
		Name: body.Name,
		Key:  []byte(body.Key),
	}, user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Delete unloads it: the owner goes back to the model of the house, and doing
// it twice is the same as doing it once.
func Delete(w http.ResponseWriter, r *http.Request) {
	service, user, ok := space.Session(w, r)
	if !ok {
		return
	}
	owner, ok := space.OwnerOf(w, r, service, user, r.URL.Query().Get("org"), true)
	if !ok {
		return
	}
	if err := service.ClearModel(r.Context(), owner, user.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func current(r *http.Request, service *app.Service, owner chat.Owner) (view, error) {
	own, err := service.OwnModel(r.Context(), owner)
	if err != nil {
		return view{}, err
	}
	return view{
		Owner:  owner,
		Own:    own.Loaded(),
		Base:   own.Base,
		Name:   own.Name,
		HasKey: len(own.Key) > 0,
		House:  service.Model,
	}, nil
}

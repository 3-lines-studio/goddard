package orgs

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/3-lines-studio/goddard/org"
	"github.com/3-lines-studio/goddard/web/app"
)

type orgView struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
	Role string `json:"role"`
}

type request struct {
	Name string `json:"name"`
}

// Get is the organizations of whoever is asking, each one with what they can do
// in it.
func Get(w http.ResponseWriter, r *http.Request) {
	service := app.Current()
	if service == nil {
		http.Error(w, "el servicio no arrancó", http.StatusServiceUnavailable)
		return
	}
	user, ok := app.Session(service, w, r)
	if !ok {
		return
	}
	orgs, err := service.Orgs.Orgs(r.Context(), user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	views := make([]orgView, 0, len(orgs))
	for _, one := range orgs {
		role, _, err := service.Orgs.Role(r.Context(), one.ID, user.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		views = append(views, orgView{ID: one.ID, Slug: one.Slug, Name: one.Name, Role: role})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"orgs": views})
}

// Post opens one, with whoever asks as its owner.
func Post(w http.ResponseWriter, r *http.Request) {
	service := app.Current()
	if service == nil {
		http.Error(w, "el servicio no arrancó", http.StatusServiceUnavailable)
		return
	}
	user, ok := app.Session(service, w, r)
	if !ok {
		return
	}
	var body request
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "no pude leer la organización", http.StatusBadRequest)
		return
	}
	created, err := service.Orgs.Create(r.Context(), body.Name, user.ID)
	if errors.Is(err, org.ErrTaken) {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(orgView{ID: created.ID, Slug: created.Slug, Name: created.Name, Role: org.RoleOwner})
}

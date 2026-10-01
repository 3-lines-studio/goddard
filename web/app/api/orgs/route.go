package orgs

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/3-lines-studio/goddard/chat"
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

// Delete takes the organization out, with its members and its projects. Only
// an owner of it asks for it, and a member who is not one gets a 403.
func Delete(w http.ResponseWriter, r *http.Request) {
	service := app.Current()
	if service == nil {
		http.Error(w, "el servicio no arrancó", http.StatusServiceUnavailable)
		return
	}
	user, ok := app.Session(service, w, r)
	if !ok {
		return
	}
	id := r.URL.Query().Get("id")
	role, in, err := service.Orgs.Role(r.Context(), id, user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !in {
		http.Error(w, org.ErrNoOrg.Error(), http.StatusNotFound)
		return
	}
	if role != org.RoleOwner {
		http.Error(w, org.ErrForbidden.Error(), http.StatusForbidden)
		return
	}
	// The projects go first: a failure after them leaves an organization
	// nobody can see, and the other way around would leave projects nobody
	// can reach.
	if err := service.Chat.DeleteProjects(r.Context(), chat.Owner{Kind: chat.OwnerOrg, ID: id}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = service.Orgs.Delete(r.Context(), id, user.ID)
	switch {
	case errors.Is(err, org.ErrForbidden):
		http.Error(w, err.Error(), http.StatusForbidden)
	case errors.Is(err, org.ErrNoOrg):
		http.Error(w, err.Error(), http.StatusNotFound)
	case err != nil:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

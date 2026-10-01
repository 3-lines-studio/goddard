package members

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/3-lines-studio/goddard/auth"
	"github.com/3-lines-studio/goddard/org"
	"github.com/3-lines-studio/goddard/web/app"
)

type memberView struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Name   string `json:"name"`
	Role   string `json:"role"`
}

type request struct {
	Org   string `json:"org"`
	Email string `json:"email"`
	User  string `json:"user"`
	Role  string `json:"role"`
}

// Get is who is in one. Anybody in it sees the rest, which is the point of
// being in the same organization.
func Get(w http.ResponseWriter, r *http.Request) {
	service, user, ok := session(w, r)
	if !ok {
		return
	}
	orgID := r.URL.Query().Get("org")
	if _, in, err := service.Orgs.Role(r.Context(), orgID, user.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	} else if !in {
		http.Error(w, org.ErrNoOrg.Error(), http.StatusNotFound)
		return
	}
	people, err := service.Orgs.Members(r.Context(), orgID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	views := make([]memberView, 0, len(people))
	for _, one := range people {
		views = append(views, memberView{UserID: one.UserID, Email: one.Email, Name: one.Name, Role: one.Role})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"members": views})
}

// Post brings somebody in by the mail they already signed in with.
func Post(w http.ResponseWriter, r *http.Request) {
	service, user, ok := session(w, r)
	if !ok {
		return
	}
	var body request
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "no pude leer el miembro", http.StatusBadRequest)
		return
	}
	if err := service.Orgs.Add(r.Context(), body.Org, body.Email, body.Role, user.ID); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Patch changes what somebody can do.
func Patch(w http.ResponseWriter, r *http.Request) {
	service, user, ok := session(w, r)
	if !ok {
		return
	}
	var body request
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "no pude leer el miembro", http.StatusBadRequest)
		return
	}
	if err := service.Orgs.SetRole(r.Context(), body.Org, body.User, body.Role, user.ID); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Delete takes somebody out.
func Delete(w http.ResponseWriter, r *http.Request) {
	service, user, ok := session(w, r)
	if !ok {
		return
	}
	query := r.URL.Query()
	if err := service.Orgs.Remove(r.Context(), query.Get("org"), query.Get("user"), user.ID); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func session(w http.ResponseWriter, r *http.Request) (*app.Service, auth.User, bool) {
	service := app.Current()
	if service == nil {
		http.Error(w, "el servicio no arrancó", http.StatusServiceUnavailable)
		return nil, auth.User{}, false
	}
	user, ok := app.Session(service, w, r)
	if !ok {
		return nil, auth.User{}, false
	}
	return service, user, true
}

// fail says what went wrong in the words of HTTP. What somebody may not do is
// 403 and not 404: pretending the organization is not there would lie about
// something they already know it is.
func fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, org.ErrForbidden):
		http.Error(w, err.Error(), http.StatusForbidden)
	case errors.Is(err, org.ErrNoOrg), errors.Is(err, org.ErrNoUser), errors.Is(err, org.ErrNoMember):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, org.ErrLastOwner):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, org.ErrRole):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

package workspace

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/3-lines-studio/goddard/auth"
	"github.com/3-lines-studio/goddard/chat"
	"github.com/3-lines-studio/goddard/heimdall"
	"github.com/3-lines-studio/goddard/org"
	"github.com/3-lines-studio/goddard/web/app"
	"github.com/3-lines-studio/goddard/workspace"
)

type request struct {
	Org  string `json:"org"`
	Path string `json:"path"`
	Addr string `json:"addr"`
	User string `json:"user"`
	Key  string `json:"key"`
}

// view is the workspace of an owner as the panel shows it. The key never comes
// back: what comes is whether there is one, because a secret that travels out
// of the store stops being one.
type view struct {
	Owner  chat.Owner `json:"owner"`
	Path   string     `json:"path"`
	Addr   string     `json:"addr"`
	User   string     `json:"user"`
	HasKey bool       `json:"has_key"`
}

// Get is where the projects of an owner live and how to reach the machine that
// runs them: the person asking, or an organization they are in.
func Get(w http.ResponseWriter, r *http.Request) {
	service, user, ok := session(w, r)
	if !ok {
		return
	}
	owner, ok := ownerOf(w, r, service, user, r.URL.Query().Get("org"), false)
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

// Post loads a workspace: where the projects live and the three secrets of the
// sandbox. What comes empty is left as it was, and the key of an organization
// is for its owners and its admins: that key opens the machine of the team.
func Post(w http.ResponseWriter, r *http.Request) {
	service, user, ok := session(w, r)
	if !ok {
		return
	}
	var body request
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "no pude leer el workspace", http.StatusBadRequest)
		return
	}
	owner, ok := ownerOf(w, r, service, user, body.Org, true)
	if !ok {
		return
	}
	err := service.SaveWorkspace(r.Context(), owner, body.Path, app.Sandbox{
		Addr: body.Addr,
		User: body.User,
		Key:  []byte(body.Key),
	}, user.ID)
	if err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func current(r *http.Request, service *app.Service, owner chat.Owner) (view, error) {
	space, err := service.Workspace(r.Context(), owner)
	if err != nil {
		return view{}, err
	}
	sandbox, err := service.Sandbox(r.Context(), owner)
	if err != nil {
		return view{}, err
	}
	return view{
		Owner:  owner,
		Path:   space.Path,
		Addr:   sandbox.Addr,
		User:   sandbox.User,
		HasKey: len(sandbox.Key) > 0,
	}, nil
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

// ownerOf is the owner a request is about: the person asking when there is no
// organization, or one they are in. Reading it is for anybody in it and
// writing it is for its owners and its admins.
func ownerOf(w http.ResponseWriter, r *http.Request, service *app.Service, user auth.User, id string, write bool) (chat.Owner, bool) {
	if id == "" {
		return chat.Owner{Kind: chat.OwnerUser, ID: user.ID}, true
	}
	role, in, err := service.Orgs.Role(r.Context(), id, user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return chat.Owner{}, false
	}
	if !in {
		http.Error(w, org.ErrNoOrg.Error(), http.StatusNotFound)
		return chat.Owner{}, false
	}
	if write && role == org.RoleMember {
		http.Error(w, org.ErrForbidden.Error(), http.StatusForbidden)
		return chat.Owner{}, false
	}
	return chat.Owner{Kind: chat.OwnerOrg, ID: id}, true
}

func fail(w http.ResponseWriter, err error) {
	var him *heimdall.Error
	switch {
	case errors.Is(err, workspace.ErrPath), errors.Is(err, workspace.ErrOwner):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.As(err, &him) && him.Kind == heimdall.ErrBad:
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

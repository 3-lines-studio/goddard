package projects

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/3-lines-studio/goddard/chat"
	"github.com/3-lines-studio/goddard/org"
	"github.com/3-lines-studio/goddard/web/app"
)

type request struct {
	Name string `json:"name"`
	Org  string `json:"org"`
}

type rename struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

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
		http.Error(w, "no pude leer el proyecto", http.StatusBadRequest)
		return
	}
	owner := chat.Owner{Kind: chat.OwnerUser, ID: user.ID}
	if body.Org != "" {
		role, in, err := service.Orgs.Role(r.Context(), body.Org, user.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !in || role != org.RoleOwner {
			http.Error(w, "esa organización no es tuya", http.StatusForbidden)
			return
		}
		owner = chat.Owner{Kind: chat.OwnerOrg, ID: body.Org}
	}
	project, err := service.Chat.CreateProject(r.Context(), body.Name, owner, user.ID)
	if errors.Is(err, chat.ErrTaken) {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(project)
}

func Patch(w http.ResponseWriter, r *http.Request) {
	service := app.Current()
	if service == nil {
		http.Error(w, "el servicio no arrancó", http.StatusServiceUnavailable)
		return
	}
	if _, ok := app.Session(service, w, r); !ok {
		return
	}
	var body rename
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "no pude leer el nombre", http.StatusBadRequest)
		return
	}
	if body.Name == "" {
		http.Error(w, "sin nombre", http.StatusBadRequest)
		return
	}
	if err := service.Chat.RenameProject(r.Context(), body.ID, body.Name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func Delete(w http.ResponseWriter, r *http.Request) {
	service := app.Current()
	if service == nil {
		http.Error(w, "el servicio no arrancó", http.StatusServiceUnavailable)
		return
	}
	if _, ok := app.Session(service, w, r); !ok {
		return
	}
	if err := service.Chat.DeleteProject(r.Context(), r.URL.Query().Get("id")); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

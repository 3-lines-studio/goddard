package conversations

import (
	"encoding/json"
	"net/http"

	"github.com/3-lines-studio/goddard/web/app"
)

type request struct {
	Project string `json:"project"`
	Title   string `json:"title"`
}

type rename struct {
	ID    string `json:"id"`
	Title string `json:"title"`
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
		http.Error(w, "no pude leer la conversación", http.StatusBadRequest)
		return
	}
	if _, ok, err := service.Chat.Project(r.Context(), body.Project); err != nil || !ok {
		http.Error(w, "ese proyecto no existe", http.StatusBadRequest)
		return
	}
	conversation, err := service.Chat.CreateConversation(r.Context(), body.Project, body.Title, "", user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(conversation)
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
		http.Error(w, "no pude leer el título", http.StatusBadRequest)
		return
	}
	if body.Title == "" {
		http.Error(w, "sin título", http.StatusBadRequest)
		return
	}
	if err := service.Chat.RenameConversation(r.Context(), body.ID, body.Title); err != nil {
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
	if err := service.Chat.DeleteConversation(r.Context(), r.URL.Query().Get("id")); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

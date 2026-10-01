package read

import (
	"encoding/json"
	"net/http"

	"github.com/3-lines-studio/goddard/web/app"
)

type request struct {
	Project string `json:"project"`
	Name    string `json:"name"`
}

// Post marks a task read, or every task of the user when the request does not
// name one: the agenda is one list and what is read there is read once.
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
		http.Error(w, "no pude leer la tarea", http.StatusBadRequest)
		return
	}
	if body.Name == "" {
		if err := service.Schedule.MarkAllRead(r.Context(), service.AgendaOf(r.Context(), user)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := service.Schedule.MarkRead(r.Context(), service.AgendaOf(r.Context(), user), body.Project, body.Name); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

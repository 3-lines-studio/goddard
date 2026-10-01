package run

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/3-lines-studio/goddard/web/app"
)

type request struct {
	Project string `json:"project"`
	Name    string `json:"name"`
}

// Post hands the task over and answers: a run takes as long as the agent takes,
// and the web is not going to hold the request for it. What the task answered
// is written in its log, which is where the agenda reads it.
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
	agenda := service.AgendaOf(r.Context(), user)
	if _, err := service.Schedule.Get(r.Context(), agenda, body.Project, body.Name); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	go func() {
		if _, err := service.Agenda.RunNow(context.Background(), agenda, body.Project, body.Name); err != nil {
			log.Printf("goddard: la corrida de %q no salió: %v", body.Name, err)
		}
	}()
	w.WriteHeader(http.StatusAccepted)
}

package memo

import (
	"encoding/json"
	"net/http"

	"github.com/3-lines-studio/goddard/web/app"
)

type factView struct {
	Key     string `json:"key"`
	Kind    string `json:"kind"`
	Body    string `json:"body"`
	Project string `json:"project"`
	Date    int64  `json:"date"`
}

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
	scope, ok := service.Scope(r.Context(), user, r.URL.Query().Get("project"))
	if !ok {
		http.Error(w, "ese proyecto no es tuyo", http.StatusNotFound)
		return
	}
	facts, err := service.Memo.Facts(r.Context(), scope)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	views := make([]factView, 0, len(facts))
	for _, fact := range facts {
		views = append(views, factView{
			Key: fact.Key, Kind: fact.Kind, Body: fact.Body, Project: fact.Project, Date: fact.Date,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"facts": views})
}

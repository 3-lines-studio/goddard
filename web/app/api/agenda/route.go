package agenda

import (
	"encoding/json"
	"net/http"

	"github.com/3-lines-studio/goddard/web/app"
)

type runView struct {
	TS   int64  `json:"ts"`
	MS   int64  `json:"ms"`
	OK   bool   `json:"ok"`
	Text string `json:"text"`
}

type taskView struct {
	Name   string   `json:"name"`
	When   string   `json:"when"`
	At     string   `json:"at"`
	Every  string   `json:"every"`
	Prompt string   `json:"prompt"`
	Paused bool     `json:"paused"`
	Silent bool     `json:"silent"`
	Target string   `json:"target"`
	Last   *runView `json:"last"`
}

type pause struct {
	Project string `json:"project"`
	Name    string `json:"name"`
	Paused  bool   `json:"paused"`
}

func Get(w http.ResponseWriter, r *http.Request) {
	service := app.Current()
	if service == nil {
		http.Error(w, "el servicio no arrancó", http.StatusServiceUnavailable)
		return
	}
	if _, ok := app.Session(service, w, r); !ok {
		return
	}
	entries, err := service.Schedule.List(r.Context(), service.Viewer.User, r.URL.Query().Get("project"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	views := make([]taskView, 0, len(entries))
	for _, entry := range entries {
		view := taskView{
			Name:   entry.Task.Name,
			When:   entry.Task.When,
			At:     entry.Task.At,
			Every:  entry.Task.Every,
			Prompt: entry.Task.Prompt,
			Paused: entry.Task.Paused,
			Silent: entry.Task.Silent,
			Target: entry.Task.Target,
		}
		if len(entry.Runs) > 0 {
			last := entry.Runs[0]
			view.Last = &runView{TS: last.TS, MS: last.MS, OK: last.OK, Text: last.Text}
		}
		views = append(views, view)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"tasks": views})
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
	var body pause
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "no pude leer la tarea", http.StatusBadRequest)
		return
	}
	if err := service.Schedule.Pause(r.Context(), service.Viewer.User, body.Project, body.Name, body.Paused); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
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
	query := r.URL.Query()
	if err := service.Schedule.Remove(r.Context(), service.Viewer.User, query.Get("project"), query.Get("name")); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

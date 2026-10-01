package agenda

import (
	"encoding/json"
	"net/http"

	"github.com/3-lines-studio/goddard/web/app"
)

// shown is how many runs of a task the agenda hands over: the store keeps more,
// and the last ones are what somebody reads.
const shown = 5

type runView struct {
	TS   int64  `json:"ts"`
	Date string `json:"date"`
	MS   int64  `json:"ms"`
	OK   bool   `json:"ok"`
	Text string `json:"text"`
}

type taskView struct {
	Name    string    `json:"name"`
	Project string    `json:"project"`
	When    string    `json:"when"`
	At      string    `json:"at"`
	Every   string    `json:"every"`
	Prompt  string    `json:"prompt"`
	Paused  bool      `json:"paused"`
	Silent  bool      `json:"silent"`
	Target  string    `json:"target"`
	Unread  int       `json:"unread"`
	Runs    []runView `json:"runs"`
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
	entries, err := service.Schedule.ListAll(r.Context(), service.Viewer.User)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	views := make([]taskView, 0, len(entries))
	for _, entry := range entries {
		view := taskView{
			Name:    entry.Task.Name,
			Project: entry.Task.Project,
			When:    entry.Task.When,
			At:      entry.Task.At,
			Every:   entry.Task.Every,
			Prompt:  entry.Task.Prompt,
			Paused:  entry.Task.Paused,
			Silent:  entry.Task.Silent,
			Target:  entry.Task.Target,
			Unread:  entry.Unread,
			Runs:    []runView{},
		}
		runs := entry.Runs
		if len(runs) > shown {
			runs = runs[len(runs)-shown:]
		}
		for index := len(runs) - 1; index >= 0; index-- {
			run := runs[index]
			view.Runs = append(view.Runs, runView{TS: run.TS, Date: run.Date, MS: run.MS, OK: run.OK, Text: run.Text})
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

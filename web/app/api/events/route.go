package events

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/3-lines-studio/goddard/web/app"
)

type line struct {
	Seq  int64           `json:"seq"`
	At   int64           `json:"at"`
	Body json.RawMessage `json:"body"`
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
	conversation := r.URL.Query().Get("conversation")
	if conversation == "" {
		http.Error(w, "falta conversation", http.StatusBadRequest)
		return
	}
	since, err := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	if err != nil {
		since = 0
	}
	events, err := service.Chat.Events(r.Context(), conversation, since)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	lines := make([]line, 0, len(events))
	for _, event := range events {
		lines = append(lines, line{Seq: event.Seq, At: event.At, Body: event.Body})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"events": lines})
}

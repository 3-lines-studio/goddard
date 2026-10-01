package stream

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/3-lines-studio/goddard/web/app"
)

const every = 300 * time.Millisecond

func Get(w http.ResponseWriter, r *http.Request) {
	service := app.Current()
	if service == nil {
		http.Error(w, "el servicio no arrancó", http.StatusServiceUnavailable)
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
	if last := r.Header.Get("Last-Event-ID"); last != "" {
		if value, err := strconv.ParseInt(last, 10, 64); err == nil && value > since {
			since = value
		}
	}
	if _, ok, err := service.Chat.Conversation(r.Context(), conversation); err != nil || !ok {
		http.Error(w, "esa conversación no existe", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	out := http.NewResponseController(w)
	if err := out.Flush(); err != nil {
		return
	}
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		events, err := service.Chat.Events(r.Context(), conversation, since)
		if err != nil {
			return
		}
		for _, event := range events {
			if _, err := fmt.Fprintf(w, "id: %d\ndata: %s\n\n", event.Seq, event.Body); err != nil {
				return
			}
			since = event.Seq
		}
		if len(events) > 0 {
			if err := out.Flush(); err != nil {
				return
			}
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

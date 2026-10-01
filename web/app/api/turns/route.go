package turns

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/3-lines-studio/goddard/web/app"
)

type request struct {
	Conversation string   `json:"conversation"`
	Text         string   `json:"text"`
	Uploads      []string `json:"uploads"`
}

func Post(w http.ResponseWriter, r *http.Request) {
	service := app.Current()
	if service == nil {
		http.Error(w, "el servicio no arrancó", http.StatusServiceUnavailable)
		return
	}
	if _, ok := app.Session(service, w, r); !ok {
		return
	}
	var body request
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "no pude leer el mensaje", http.StatusBadRequest)
		return
	}
	err := service.Say(r.Context(), body.Conversation, body.Text, body.Uploads)
	switch {
	case err == nil:
		w.WriteHeader(http.StatusAccepted)
	case errors.Is(err, app.ErrBusy):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, app.ErrNoConversation), errors.Is(err, app.ErrReadOnly), errors.Is(err, app.ErrEmpty):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

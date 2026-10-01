package stop

import (
	"encoding/json"
	"net/http"

	"github.com/3-lines-studio/goddard/web/app"
)

type request struct {
	Conversation string `json:"conversation"`
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
		http.Error(w, "no pude leer la conversación", http.StatusBadRequest)
		return
	}
	if !service.Stop(body.Conversation) {
		http.Error(w, "no hay ningún turno corriendo en esa conversación", http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

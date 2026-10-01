package login

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/3-lines-studio/goddard/web/app"
)

type request struct {
	Email string `json:"email"`
}

func Post(w http.ResponseWriter, r *http.Request) {
	service := app.Current()
	if service == nil {
		http.Error(w, "el servicio no arrancó", http.StatusServiceUnavailable)
		return
	}
	var body request
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "no pude leer el mail", http.StatusBadRequest)
		return
	}
	token, err := service.Login(r.Context(), body.Email)
	if errors.Is(err, app.ErrNotAllowed) {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if errors.Is(err, app.ErrTooSoon) {
		http.Error(w, err.Error(), http.StatusTooManyRequests)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	link := app.Base(r) + "/auth?token=" + token
	sent, err := service.SendLink(r.Context(), body.Email, link)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if !sent {
		_ = json.NewEncoder(w).Encode(map[string]any{"sent": false, "link": link})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"sent": true})
}

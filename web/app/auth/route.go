package auth

import (
	"net/http"

	"github.com/3-lines-studio/goddard/web/app"
)

func Get(w http.ResponseWriter, r *http.Request) {
	service := app.Current()
	if service == nil {
		http.Error(w, "el servicio no arrancó", http.StatusServiceUnavailable)
		return
	}
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "falta el token", http.StatusBadRequest)
		return
	}
	_, session, err := service.Open(r.Context(), token)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	app.SetCookie(w, r, session, 30*24*60*60)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

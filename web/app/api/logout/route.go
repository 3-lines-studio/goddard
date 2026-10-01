package logout

import (
	"net/http"

	"github.com/3-lines-studio/goddard/web/app"
)

func Post(w http.ResponseWriter, r *http.Request) {
	service := app.Current()
	if service == nil {
		http.Error(w, "el servicio no arrancó", http.StatusServiceUnavailable)
		return
	}
	service.Close(r.Context(), r)
	app.SetCookie(w, r, "", -1)
	w.WriteHeader(http.StatusNoContent)
}

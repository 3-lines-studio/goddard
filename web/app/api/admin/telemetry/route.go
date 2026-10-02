package telemetry

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/3-lines-studio/goddard/web/app"
)

// MaxDays is how far back the window may reach: the numbers of a year are
// still numbers, and more than that is a data warehouse.
const MaxDays = 365

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
	if !service.Admin(user) {
		http.Error(w, "esto es de la casa", http.StatusForbidden)
		return
	}
	days, ok := window(r)
	if !ok {
		http.Error(w, "days tiene que ser un número entre 1 y "+strconv.Itoa(MaxDays), http.StatusBadRequest)
		return
	}
	until := time.Now().Unix() + 1
	from := until - days*24*3600
	summary, err := service.Metric.Summary(r.Context(), from, until, service.Offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(summary)
}

// window is how many days back the answer reaches: a week unless the request
// says otherwise.
func window(r *http.Request) (int64, bool) {
	raw := r.URL.Query().Get("days")
	if raw == "" {
		return 7, true
	}
	days, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || days < 1 || days > MaxDays {
		return 0, false
	}
	return days, true
}

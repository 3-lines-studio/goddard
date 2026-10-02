package app

import (
	"errors"
	"net/http"

	"github.com/3-lines-studio/bifrost"
)

// Load keeps the app for whoever has a session: without one there is nothing
// to show, so the request goes to the login, which is where a link is asked
// for and where the cookie comes from.
func Load(r *http.Request) (any, error) {
	service := Current()
	if service == nil {
		return nil, errors.New("goddard: el servicio no arrancó")
	}
	if _, ok := service.Who(r.Context(), r); !ok {
		return nil, bifrost.Redirect("/login", http.StatusSeeOther)
	}
	return map[string]string{}, nil
}

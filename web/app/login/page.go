package login

import (
	"errors"
	"net/http"

	"github.com/3-lines-studio/bifrost"

	"github.com/3-lines-studio/goddard/web/app"
)

// Load leaves the login for whoever has no session: somebody who already came
// in goes home.
func Load(r *http.Request) (any, error) {
	service := app.Current()
	if service == nil {
		return nil, errors.New("goddard: el servicio no arrancó")
	}
	if _, ok := service.Who(r.Context(), r); ok {
		return nil, bifrost.Redirect("/", http.StatusSeeOther)
	}
	return map[string]string{}, nil
}

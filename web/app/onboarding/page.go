package onboarding

import (
	"errors"
	"net/http"

	"github.com/3-lines-studio/bifrost"

	"github.com/3-lines-studio/goddard/chat"
	"github.com/3-lines-studio/goddard/web/app"
)

// Load keeps the first setup for whoever has a session and still owes it:
// without a session the request goes to the login, and somebody whose goddard
// already works goes home.
func Load(r *http.Request) (any, error) {
	service := app.Current()
	if service == nil {
		return nil, errors.New("goddard: el servicio no arrancó")
	}
	user, ok := service.Who(r.Context(), r)
	if !ok {
		return nil, bifrost.Redirect("/login", http.StatusSeeOther)
	}
	setup, err := service.Onboarding(r.Context(), chat.Owner{Kind: chat.OwnerUser, ID: user.ID})
	if err != nil {
		return nil, err
	}
	if setup.Done() {
		return nil, bifrost.Redirect("/", http.StatusSeeOther)
	}
	return map[string]string{}, nil
}

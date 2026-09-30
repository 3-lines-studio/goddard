package heimdall

import (
	"errors"
	"fmt"
)

// Actor is who is asking: the administrator, or a token with its scope.
type Actor struct {
	Admin bool
	Name  string
	Token *Token
}

// StatusError is a refusal with the status the caller should answer with. The
// message is what goes to the client; Cause, when there is one, is for the log
// and never for the response.
type StatusError struct {
	Status  int
	Message string
	Cause   error
}

func (e *StatusError) Error() string {
	return e.Message
}

// Refuse maps what the store returned onto an answer: the store's bad input is
// the client's error, and its internal failures are a 500 with a message that
// gives nothing away.
func Refuse(err error) *StatusError {
	var typed *Error
	if errors.As(err, &typed) && typed.Kind == ErrInternal {
		return Internal(err)
	}
	return &StatusError{Status: 400, Message: err.Error()}
}

func Internal(cause error) *StatusError {
	return &StatusError{Status: 500, Message: "algo se rompió acá adentro", Cause: cause}
}

// Authenticate takes the session cookie, the bearer token and the service's own
// admin token. The cookie and the bearer are already split out by whoever
// parsed the request: this does not know about HTTP.
func Authenticate(store *Store, cookie, bearer, adminToken string) (Actor, *StatusError) {
	if cookie != "" {
		email, found, err := store.Session(cookie)
		if err != nil {
			return Actor{}, Internal(err)
		}
		if found {
			return Actor{Admin: true, Name: email}, nil
		}
	}
	if bearer == "" {
		return Actor{}, unauthorized("falta el token")
	}
	if Equal(bearer, adminToken) {
		return Actor{Admin: true, Name: "admin"}, nil
	}
	token, err := store.Find(bearer)
	if err != nil {
		var typed *Error
		if errors.As(err, &typed) && typed.Kind == ErrBad {
			return Actor{}, unauthorized(typed.Message)
		}
		return Actor{}, Internal(err)
	}
	if token == nil {
		return Actor{}, unauthorized("ese token no sirve")
	}
	store.Touch(token.ID)
	if token.Admin {
		return Actor{Admin: true, Name: token.Name}, nil
	}
	return Actor{Name: token.Name, Token: token}, nil
}

// ScopedRequest is the whole guard of a read: the scope has to be there and the
// actor has to reach it.
func (a Actor) ScopedRequest(project, env string) *StatusError {
	if project == "" || env == "" {
		return Refuse(bad("faltan project y env"))
	}
	return a.Allows(project, env)
}

func (a Actor) Allows(project, env string) *StatusError {
	if a.Admin {
		return nil
	}
	if a.Token != nil && covers(a.Token.Project, project) && covers(a.Token.Env, env) {
		return nil
	}
	return forbidden("este token no llega a ese entorno")
}

func (a Actor) AllowsAdmin() *StatusError {
	if a.Admin {
		return nil
	}
	return forbidden("este token no administra")
}

func (a Actor) AllowsKey(name string) *StatusError {
	if a.Admin || a.Token == nil || a.Token.Keys == nil || name == "" {
		return nil
	}
	for _, key := range a.Token.Keys {
		if key == name {
			return nil
		}
	}
	return forbidden(fmt.Sprintf("este token no ve %s", name))
}

func (a Actor) Visible(secrets map[string]string) map[string]string {
	if a.Admin || a.Token == nil || a.Token.Keys == nil {
		return secrets
	}
	visible := map[string]string{}
	for name, value := range secrets {
		if a.AllowsKey(name) == nil {
			visible[name] = value
		}
	}
	return visible
}

func covers(pattern, value string) bool {
	return pattern == "*" || pattern == value
}

func unauthorized(message string) *StatusError {
	return &StatusError{Status: 401, Message: message}
}

func forbidden(message string) *StatusError {
	return &StatusError{Status: 403, Message: message}
}

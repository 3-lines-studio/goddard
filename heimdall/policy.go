package heimdall

import (
	"context"
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

// Authenticate takes who the user says they are — already resolved from the
// session cookie by whoever reads it — the bearer token and heimdall's own
// admin token. The cookie is not heimdall's business anymore, and neither is
// HTTP: the caller splits its request and passes the name.
func Authenticate(ctx context.Context, store *Store, user, bearer, adminToken string) (Actor, *StatusError) {
	if user != "" {
		return Actor{Admin: true, Name: user}, nil
	}
	if bearer == "" {
		return Actor{}, unauthorized("falta el token")
	}
	if Equal(bearer, adminToken) {
		return Actor{Admin: true, Name: "admin"}, nil
	}
	token, err := store.Find(ctx, bearer)
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
	store.Touch(ctx, token.ID)
	if token.Role == RoleAdmin {
		return Actor{Admin: true, Name: token.Name}, nil
	}
	return Actor{Name: token.Name, Token: token}, nil
}

// ScopedRequest is the whole guard of a read: the scope has to be there and the
// actor has to reach it.
func (a Actor) ScopedRequest(owner Owner, project, env string) *StatusError {
	if project == "" || env == "" {
		return Refuse(bad("faltan project y env"))
	}
	return a.Allows(owner, project, env)
}

func (a Actor) Allows(owner Owner, project, env string) *StatusError {
	if a.Admin {
		return nil
	}
	if a.Token != nil && a.Token.Owner == owner && covers(a.Token.Project, project) && covers(a.Token.Env, env) {
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

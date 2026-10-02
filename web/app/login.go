package app

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/3-lines-studio/goddard/auth"
)

// Cookie is the session cookie of goddard.
const Cookie = "goddard_session"

// Cooldown is how long somebody has to wait between asking for two links.
const Cooldown = 60

var (
	ErrNotAllowed = errors.New("ese mail no está en la lista")
	ErrTooSoon    = errors.New("pediste un link hace un momento")
	ErrNoSession  = errors.New("no hay sesión")
	ErrNoMailer   = errors.New("no hay proveedor de mails: falta RESEND_API_KEY o GODDARD_WEB_FROM")
)

// Login mints a one-shot link for an email and returns what goes in it. It is
// the caller who mails it, and the link never travels back in the answer.
func (s *Service) Login(ctx context.Context, email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if !strings.Contains(email, "@") {
		return "", errors.New("ese mail no parece un mail")
	}
	if !s.allowed(email) {
		return "", ErrNotAllowed
	}
	asked, err := s.Auth.AskedRecently(ctx, email, Cooldown)
	if err != nil {
		return "", err
	}
	if asked {
		return "", ErrTooSoon
	}
	return s.Auth.CreateLogin(ctx, email, auth.LoginTTL)
}

// Open burns a link and opens the session that goes in the cookie.
func (s *Service) Open(ctx context.Context, token string) (auth.User, string, error) {
	user, err := s.Auth.ConsumeLogin(ctx, token)
	if err != nil {
		return auth.User{}, "", err
	}
	session, err := s.Auth.CreateSession(ctx, user.ID, auth.SessionTTL)
	if err != nil {
		return auth.User{}, "", err
	}
	return user, session, nil
}

// Who is the user behind the cookie of this request, if there is one.
func (s *Service) Who(ctx context.Context, r *http.Request) (auth.User, bool) {
	cookie, err := r.Cookie(Cookie)
	if err != nil {
		return auth.User{}, false
	}
	user, ok, err := s.Auth.Session(ctx, cookie.Value)
	if err != nil {
		return auth.User{}, false
	}
	return user, ok
}

// Close signs the session of this request out.
func (s *Service) Close(ctx context.Context, r *http.Request) {
	cookie, err := r.Cookie(Cookie)
	if err != nil {
		return
	}
	_ = s.Auth.DropSession(ctx, cookie.Value)
}

// SetCookie leaves the session in the browser. Secure only when the request
// came over TLS, which is how it is behind the proxy and how it is not on
// localhost.
func SetCookie(w http.ResponseWriter, r *http.Request, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     Cookie,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure(r),
		MaxAge:   maxAge,
	})
}

func secure(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

// Base is where this request is being served from, so a link in a mail points
// wherever the person asking is.
func Base(r *http.Request) string {
	scheme := "http"
	if secure(r) {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// allowed is the list of emails that may ask for a link. An empty list lets
// anybody in, which is what a goddard of one is.
func (s *Service) allowed(email string) bool {
	if len(s.Allowed) == 0 {
		return true
	}
	for _, one := range s.Allowed {
		if strings.EqualFold(strings.TrimSpace(one), email) {
			return true
		}
	}
	return false
}

// SendLink mails a link when there is a provider. Without one there is nothing
// to send it with, so the link is left in the log of the server and never in
// the answer: a link handed back is a way in for whoever asks, and a goddard
// that lost its mailer by accident would be open to anybody.
func (s *Service) SendLink(ctx context.Context, to, link string) error {
	if s.Mail == nil {
		log.Printf("goddard: sin proveedor de mails, el link de %s es %s", to, link)
		return ErrNoMailer
	}
	return s.Mail.sendLink(ctx, to, link, s.Assistant)
}

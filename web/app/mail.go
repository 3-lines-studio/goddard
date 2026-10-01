package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

const mailTimeout = 20 * time.Second

type mailer struct {
	key      string
	from     string
	endpoint string
	client   *http.Client
}

// newMailer is nil when there is no provider: without a key the link is handed
// back instead of mailed, which is how it is used in development.
func newMailer() *mailer {
	key := os.Getenv("RESEND_API_KEY")
	from := env("GODDARD_WEB_FROM", os.Getenv("JIMMY_WEB_FROM"))
	if key == "" || from == "" {
		return nil
	}
	base := env("RESEND_API_BASE", "https://api.resend.com")
	return &mailer{
		key:      key,
		from:     from,
		endpoint: base + "/emails",
		client:   &http.Client{Timeout: mailTimeout},
	}
}

func (m *mailer) sendLink(ctx context.Context, to, link, assistant string) error {
	html := fmt.Sprintf(
		"<p>Entrá a %s con este link:</p><p><a href=\"%s\">%s</a></p>"+
			"<p>Vence en unos minutos y sirve una sola vez. Si no lo pediste vos, ignoralo.</p>",
		assistant, link, link)
	body, err := json.Marshal(map[string]any{
		"from":    m.from,
		"to":      []string{to},
		"subject": "Tu link para entrar a " + assistant,
		"html":    html,
	})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, m.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+m.key)
	request.Header.Set("Content-Type", "application/json")
	response, err := m.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 {
		return fmt.Errorf("resend contestó %s", response.Status)
	}
	return nil
}

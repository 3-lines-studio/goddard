package compute

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// BoatURL is where the public API of boat lives.
const BoatURL = "https://boat.dev/api/v1"

// Boat is the house: it asks boat for a machine, which is a Ubuntu VM with SSH
// and a disk that survives being stopped, and hands the address over so the
// turn can go in with compute.SSH. The token is the account's, and a sandbox is
// asked with no environment so it holds none of the account's secrets.
type Boat struct {
	url   string
	token string
	http  *http.Client
}

func NewBoat(token string) *Boat {
	return &Boat{url: BoatURL, token: token, http: &http.Client{}}
}

func (b *Boat) Create(ctx context.Context, spec Spec) (Sandbox, error) {
	body := map[string]any{"noEnv": true}
	if spec.Size != "" {
		body["type"] = spec.Size
	}
	if spec.TTL > 0 {
		body["ttlSeconds"] = int64(spec.TTL.Seconds())
	} else {
		body["ttlSeconds"] = nil
	}
	if spec.Setup != "" {
		body["setupScript"] = spec.Setup
	}
	var out struct {
		Sandbox boatSandbox `json:"sandbox"`
	}
	if err := b.do(ctx, http.MethodPost, "/sandboxes", body, &out); err != nil {
		return Sandbox{}, err
	}
	return out.Sandbox.value(), nil
}

func (b *Boat) Get(ctx context.Context, id string) (Sandbox, error) {
	var out struct {
		Sandbox boatSandbox `json:"sandbox"`
	}
	if err := b.do(ctx, http.MethodGet, "/sandboxes/"+id, nil, &out); err != nil {
		return Sandbox{}, err
	}
	return out.Sandbox.value(), nil
}

func (b *Boat) Stop(ctx context.Context, id string) error {
	return b.do(ctx, http.MethodPost, "/sandboxes/"+id+"/stop", nil, nil)
}

func (b *Boat) Resume(ctx context.Context, id string) error {
	return b.do(ctx, http.MethodPost, "/sandboxes/"+id+"/resume", nil, nil)
}

// Delete is not Stop: the sandbox cannot be resumed again. Boat asks for the id
// as a confirmation header, and it is the same id twice on purpose.
func (b *Boat) Delete(ctx context.Context, id string) error {
	return b.send(ctx, http.MethodDelete, "/sandboxes/"+id, nil, nil, map[string]string{"X-Ascii-Confirm-Delete": id})
}

// Authorize leaves the public key on the machine so the turn can get in. The
// private half never leaves heimdall.
func (b *Boat) Authorize(ctx context.Context, id string, publicKey string) error {
	body := map[string]any{"key": publicKey}
	return b.do(ctx, http.MethodPost, "/sandboxes/"+id+"/sshkey", body, nil)
}

// Host puts a port of the machine behind a public HTTPS URL of its own, which
// is what a preview needs: nothing to rewrite, because nothing lives behind a
// path.
func (b *Boat) Host(ctx context.Context, id string, port int) (string, error) {
	var out struct {
		URL string `json:"url"`
	}
	if err := b.do(ctx, http.MethodPost, "/sandboxes/"+id+"/host", map[string]any{"port": port}, &out); err != nil {
		return "", err
	}
	return out.URL, nil
}

// Wait is what Create alone does not do: a sandbox answers provisioning first
// and ready a moment later. It gives up when the context does.
func (b *Boat) Wait(ctx context.Context, id string) (Sandbox, error) {
	for {
		one, err := b.Get(ctx, id)
		if err != nil {
			return Sandbox{}, err
		}
		switch one.State {
		case "ready", "idle", "running":
			return one, nil
		case "error", "cancelled":
			return Sandbox{}, fmt.Errorf("boat: el sandbox %s quedó en %s", id, one.State)
		}
		select {
		case <-ctx.Done():
			return Sandbox{}, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func (b *Boat) do(ctx context.Context, method, path string, body any, out any) error {
	return b.send(ctx, method, path, body, out, nil)
}

func (b *Boat) send(ctx context.Context, method, path string, body any, out any, headers map[string]string) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, b.url+path, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+b.token)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := b.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	content, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return boatFailure(response.StatusCode, content)
	}
	if out == nil || len(content) == 0 {
		return nil
	}
	return json.Unmarshal(content, out)
}

// boatFailure is the error envelope of the API: the code says what went wrong
// and the message says it in a sentence, which is what a log wants.
func boatFailure(status int, content []byte) error {
	var envelope struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(content, &envelope)
	if envelope.Code == "" && envelope.Message == "" {
		return fmt.Errorf("boat: la API contestó %d", status)
	}
	return fmt.Errorf("boat: %s (%s, %d)", envelope.Message, envelope.Code, status)
}

type boatSandbox struct {
	ID          string `json:"id"`
	State       string `json:"state"`
	IP          string `json:"ip"`
	SSHEndpoint string `json:"sshEndpoint"`
}

func (s boatSandbox) value() Sandbox {
	addr := s.IP
	if s.SSHEndpoint != "" {
		addr = s.SSHEndpoint
	}
	return Sandbox{ID: s.ID, State: s.State, Addr: addr, User: "user"}
}

var _ Provider = (*Boat)(nil)

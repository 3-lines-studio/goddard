package compute

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeBoat is the API of boat: it keeps what the test asked it and answers what
// a real one answers, so the client is measured without a token or a bill.
type fakeBoat struct {
	*httptest.Server
	paths   []string
	bodies  []map[string]any
	headers []http.Header
}

func newFakeBoat(t *testing.T) (*Boat, *fakeBoat) {
	t.Helper()
	fake := &fakeBoat{}
	fake.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(content, &body)
		fake.paths = append(fake.paths, r.Method+" "+r.URL.Path)
		fake.bodies = append(fake.bodies, body)
		fake.headers = append(fake.headers, r.Header.Clone())
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/sandboxes":
			w.WriteHeader(http.StatusAccepted)
			io.WriteString(w, `{"ok":true,"type":"sandbox.created","status":"provisioning","ttlSeconds":3600,
				"sandbox":{"id":"bx_23456789","name":"Boat","state":"provisioning","ip":null}}`)
		case r.URL.Path == "/sandboxes/bx_23456789":
			io.WriteString(w, `{"ok":true,"sandbox":{"id":"bx_23456789","state":"ready","ip":"10.0.0.9","sshEndpoint":"203.0.113.10:22001"}}`)
		case strings.HasSuffix(r.URL.Path, "/sshkey"), strings.HasSuffix(r.URL.Path, "/stop"), strings.HasSuffix(r.URL.Path, "/resume"):
			io.WriteString(w, `{"ok":true,"type":"sandbox.updated"}`)
		case strings.HasSuffix(r.URL.Path, "/host"):
			io.WriteString(w, `{"ok":true,"url":"https://frazil-pneuma-rallye-3000.on.boat.dev","isProtected":true}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"ok":false,"status":404,"code":"not_found","message":"no está"}`)
		}
	}))
	t.Cleanup(fake.Close)
	boat := NewBoat("boat_de_prueba")
	boat.url = fake.URL
	return boat, fake
}

func TestBoatAsksForAMachineWithoutTheSecretsOfTheAccount(t *testing.T) {
	boat, fake := newFakeBoat(t)
	one, err := boat.Create(t.Context(), Spec{Size: "small", TTL: 2 * time.Hour, Setup: "apt-get install -y git"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if one.ID != "bx_23456789" || one.State != "provisioning" {
		t.Fatalf("volvió %+v", one)
	}
	body := fake.bodies[0]
	if body["noEnv"] != true || body["type"] != "small" || body["ttlSeconds"] != float64(7200) {
		t.Fatalf("pidió %+v", body)
	}
	if body["setupScript"] != "apt-get install -y git" {
		t.Fatalf("el script quedó %q", body["setupScript"])
	}
	if got := fake.headers[0].Get("Authorization"); got != "Bearer boat_de_prueba" {
		t.Fatalf("mandó %q", got)
	}
}

func TestBoatWaitsUntilTheMachineIsReady(t *testing.T) {
	boat, _ := newFakeBoat(t)
	one, err := boat.Wait(t.Context(), "bx_23456789")
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if one.State != "ready" {
		t.Fatalf("quedó %+v", one)
	}
	if one.Addr != "203.0.113.10:22001" || one.User != "user" {
		t.Fatalf("la dirección quedó %q para %q", one.Addr, one.User)
	}
}

func TestBoatLeavesTheKeyThatOpensTheMachine(t *testing.T) {
	boat, fake := newFakeBoat(t)
	if err := boat.Authorize(t.Context(), "bx_23456789", "ssh-ed25519 AAAAC3Nza usuario@casa"); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if fake.paths[0] != "POST /sandboxes/bx_23456789/sshkey" {
		t.Fatalf("llamó a %s", fake.paths[0])
	}
	if fake.bodies[0]["key"] != "ssh-ed25519 AAAAC3Nza usuario@casa" {
		t.Fatalf("mandó %+v", fake.bodies[0])
	}
}

func TestBoatStopsAndResumesTheSameMachine(t *testing.T) {
	boat, fake := newFakeBoat(t)
	if err := boat.Stop(t.Context(), "bx_23456789"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if err := boat.Resume(t.Context(), "bx_23456789"); err != nil {
		t.Fatalf("resume: %v", err)
	}
	want := []string{"POST /sandboxes/bx_23456789/stop", "POST /sandboxes/bx_23456789/resume"}
	for i, one := range want {
		if fake.paths[i] != one {
			t.Fatalf("llamó a %s y esperaba %s", fake.paths[i], one)
		}
	}
}

func TestBoatDeletesOnlyWithTheIdAsConfirmation(t *testing.T) {
	boat, fake := newFakeBoat(t)
	if err := boat.Delete(t.Context(), "bx_23456789"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if fake.paths[0] != "DELETE /sandboxes/bx_23456789" {
		t.Fatalf("llamó a %s", fake.paths[0])
	}
	if got := fake.headers[0].Get("X-Ascii-Confirm-Delete"); got != "bx_23456789" {
		t.Fatalf("confirmó con %q", got)
	}
}

func TestBoatPutsAPortBehindAUrlOfItsOwn(t *testing.T) {
	boat, fake := newFakeBoat(t)
	url, err := boat.Host(t.Context(), "bx_23456789", 3000)
	if err != nil {
		t.Fatalf("host: %v", err)
	}
	if url != "https://frazil-pneuma-rallye-3000.on.boat.dev" {
		t.Fatalf("quedó %q", url)
	}
	if fake.bodies[0]["port"] != float64(3000) {
		t.Fatalf("pidió %+v", fake.bodies[0])
	}
}

func TestBoatSaysWhatTheApiSaid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		io.WriteString(w, `{"ok":false,"status":402,"code":"billing_required","message":"Your wallet is empty.","error":{}}`)
	}))
	t.Cleanup(server.Close)
	boat := NewBoat("boat_de_prueba")
	boat.url = server.URL
	_, err := boat.Create(context.Background(), Spec{})
	if err == nil || !strings.Contains(err.Error(), "billing_required") || !strings.Contains(err.Error(), "wallet is empty") {
		t.Fatalf("el error quedó %v", err)
	}
}

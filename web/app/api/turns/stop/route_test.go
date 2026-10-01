package stop

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/3-lines-studio/goddard/web/app"
	"github.com/3-lines-studio/goddard/web/app/apptest"
)

// slow answers one token and then waits, which is a turn that takes its time.
// It says so on started, so a test can stop it in the middle instead of
// guessing when it began.
func slow(t *testing.T) (*httptest.Server, <-chan struct{}, func()) {
	t.Helper()
	release := make(chan struct{})
	started := make(chan struct{})
	letGo := sync.OnceFunc(func() { close(release) })
	began := sync.OnceFunc(func() { close(started) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"uno\"}}]}\n\n")
		flusher.Flush()
		began()
		<-release
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(func() { letGo(); server.Close() })
	return server, started, letGo
}

func stoppedRun(t *testing.T, server *httptest.Server, cookie *http.Cookie, conversation string) {
	t.Helper()
	service := app.Current()
	recorder := httptest.NewRecorder()
	Post(recorder, apptest.Request(t, "POST", "/api/turns/stop", map[string]string{"conversation": conversation}, cookie))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("detener contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	apptest.Wait(t, service, conversation)
	got := apptest.Events(t, service, conversation)
	if len(got) != 3 {
		t.Fatalf("el log quedó %v", got)
	}
	for index, esperado := range []string{"user", "stopped", "done"} {
		if event := apptest.Event(t, got[index]); event["event"] != esperado {
			t.Fatalf("la línea %d quedó %s", index, got[index])
		}
	}
}

func TestPostWantsASession(t *testing.T) {
	apptest.Route(t, apptest.Provider(t))
	recorder := httptest.NewRecorder()
	Post(recorder, apptest.Request(t, "POST", "/api/turns/stop", map[string]string{"conversation": "c"}, nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("sin sesión contestó %d", recorder.Code)
	}
}

func TestPostSaysWhenThereIsNothingToStop(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, "berti@ejemplo.com")
	conversation := apptest.Thread(t, service)
	recorder := httptest.NewRecorder()
	Post(recorder, apptest.Request(t, "POST", "/api/turns/stop", map[string]string{"conversation": conversation.ID}, cookie))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("sin turno contestó %d", recorder.Code)
	}
}

func TestPostCutsTheTurnThatIsRunning(t *testing.T) {
	server, started, letGo := slow(t)
	service := apptest.Route(t, server)
	cookie := apptest.Session(t, service, "berti@ejemplo.com")
	conversation := apptest.Thread(t, service)
	if err := service.Say(t.Context(), conversation.ID, "contame algo largo", nil); err != nil {
		t.Fatalf("say: %v", err)
	}
	<-started
	stoppedRun(t, server, cookie, conversation.ID)
	letGo()
}

func TestPostCutsATurnThatHasNotStartedYet(t *testing.T) {
	server, _, letGo := slow(t)
	service := apptest.Route(t, server)
	cookie := apptest.Session(t, service, "berti@ejemplo.com")
	conversation := apptest.Thread(t, service)
	if err := service.Say(t.Context(), conversation.ID, "contame algo largo", nil); err != nil {
		t.Fatalf("say: %v", err)
	}
	stoppedRun(t, server, cookie, conversation.ID)
	letGo()
}

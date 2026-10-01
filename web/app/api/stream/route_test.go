package stream

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/3-lines-studio/goddard/web/app/apptest"
)

func TestStreamWantsASession(t *testing.T) {
	apptest.Route(t, apptest.Provider(t))
	recorder := httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/stream?conversation=c", nil, nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("sin sesión contestó %d", recorder.Code)
	}
}

func TestStreamSendsTheLogAsEvents(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t, []string{
		`{"choices":[{"delta":{"content":"hola"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	}))
	cookie := apptest.Session(t, service, "berti@ejemplo.com")
	conversation := apptest.Thread(t, service)
	if err := service.Say(t.Context(), conversation.ID, "qué tal", nil); err != nil {
		t.Fatalf("say: %v", err)
	}
	apptest.Wait(t, service, conversation.ID)

	turn, cancel := context.WithCancel(t.Context())
	request := apptest.Request(t, "GET", "/api/stream?conversation="+conversation.ID, nil, cookie).WithContext(turn)
	recorder := httptest.NewRecorder()
	go func() {
		time.Sleep(500 * time.Millisecond)
		cancel()
	}()
	Get(recorder, request)

	if got := recorder.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("el tipo quedó %q", got)
	}
	body := recorder.Body.String()
	for _, esperado := range []string{"id: 1\ndata: ", `"event": "user"`, `"event": "done"`} {
		if !strings.Contains(body, esperado) {
			t.Fatalf("el cuerpo no tiene %q:\n%s", esperado, body)
		}
	}
}

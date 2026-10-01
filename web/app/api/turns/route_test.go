package turns

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/3-lines-studio/goddard/web/app"
	"github.com/3-lines-studio/goddard/web/app/apptest"
)

func TestPostWantsASession(t *testing.T) {
	apptest.Route(t, apptest.Provider(t))
	recorder := httptest.NewRecorder()
	Post(recorder, apptest.Request(t, "POST", "/api/turns", map[string]string{"conversation": "c", "text": "hola"}, nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("sin sesión contestó %d", recorder.Code)
	}
}

func TestPostTakesTheMessageAndLeavesItInTheLog(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t, []string{
		`{"choices":[{"delta":{"content":"cuarenta y dos"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	}))
	cookie := apptest.Session(t, service, "berti@ejemplo.com")
	conversation := apptest.Thread(t, service)

	recorder := httptest.NewRecorder()
	Post(recorder, apptest.Request(t, "POST", "/api/turns", map[string]string{
		"conversation": conversation.ID,
		"text":         "cuánto es 6*7",
	}, cookie))
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	apptest.Wait(t, service, conversation.ID)
	got := apptest.Events(t, service, conversation.ID)
	if len(got) != 3 {
		t.Fatalf("el log quedó %v", got)
	}
	if first := apptest.Event(t, got[0]); first["event"] != "user" || first["text"] != "cuánto es 6*7" {
		t.Fatalf("primera línea: %s", got[0])
	}
}

func TestPostRejectsWhatItCannotDo(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, "berti@ejemplo.com")
	conversation := apptest.Thread(t, service)
	for _, caso := range []struct {
		nombre string
		body   map[string]string
	}{
		{"una conversación que no existe", map[string]string{"conversation": "no-existe", "text": "hola"}},
		{"un mensaje vacío", map[string]string{"conversation": conversation.ID, "text": "  "}},
	} {
		recorder := httptest.NewRecorder()
		Post(recorder, apptest.Request(t, "POST", "/api/turns", caso.body, cookie))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s contestó %d", caso.nombre, recorder.Code)
		}
	}
}

func TestPostSaysWhenTheConversationIsBusy(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, "berti@ejemplo.com")
	conversation := apptest.Thread(t, service)
	if taken, err := service.Chat.Claim(t.Context(), conversation.ID, app.TurnLease); err != nil || !taken {
		t.Fatalf("no pude tomar la conversación: %v %v", taken, err)
	}
	recorder := httptest.NewRecorder()
	Post(recorder, apptest.Request(t, "POST", "/api/turns", map[string]string{
		"conversation": conversation.ID,
		"text":         "hola",
	}, cookie))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("ocupada contestó %d", recorder.Code)
	}
}

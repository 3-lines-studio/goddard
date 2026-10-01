package events

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/3-lines-studio/goddard/web/app/apptest"
)

func TestEventsWantsASession(t *testing.T) {
	apptest.Route(t, apptest.Provider(t))
	recorder := httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/events?conversation=c&since=0", nil, nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("sin sesión contestó %d", recorder.Code)
	}
}

func TestEventsIsTheLogFromTheSeqItIsAsked(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t, []string{
		`{"choices":[{"delta":{"content":"hola"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	}))
	cookie := apptest.Session(t, service, "berti@ejemplo.com")
	conversation := apptest.Thread(t, service)
	if err := service.Say(t.Context(), conversation.ID, apptest.User(t, service), "qué tal", nil); err != nil {
		t.Fatalf("say: %v", err)
	}
	apptest.Wait(t, service, conversation.ID)

	recorder := httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/events?conversation="+conversation.ID+"&since=0", nil, cookie))
	if recorder.Code != http.StatusOK {
		t.Fatalf("contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	var body struct {
		Events []struct {
			Seq  int64          `json:"seq"`
			Body map[string]any `json:"body"`
		} `json:"events"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("no pude leer la respuesta: %v", err)
	}
	if len(body.Events) != 3 {
		t.Fatalf("llegaron %d eventos", len(body.Events))
	}

	recorder = httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/events?conversation="+conversation.ID+"&since=1", nil, cookie))
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("no pude leer la respuesta: %v", err)
	}
	if len(body.Events) != 2 || body.Events[0].Seq != 2 {
		t.Fatalf("desde 1 llegaron %+v", body.Events)
	}
}

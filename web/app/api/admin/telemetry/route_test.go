package telemetry

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/3-lines-studio/goddard/metric"
	"github.com/3-lines-studio/goddard/web/app/apptest"
)

func TestTelemetryWantsASession(t *testing.T) {
	apptest.Route(t, apptest.Provider(t))
	recorder := httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/admin/telemetry", nil, nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("sin sesión contestó %d", recorder.Code)
	}
}

func TestTelemetryIsOnlyForTheHouse(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	service.Admins = []string{"casa@ejemplo.com"}
	cookie := apptest.Session(t, service, "berti@ejemplo.com")
	recorder := httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/admin/telemetry", nil, cookie))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("contestó %d", recorder.Code)
	}
}

func TestTelemetryAddsUpTheTurnsAndNoNames(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t,
		[]string{
			`{"choices":[{"delta":{"content":"hola"}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
			`{"choices":[],"usage":{"prompt_tokens":12,"completion_tokens":3}}`,
		},
		[]string{
			`{"choices":[{"delta":{"content":"chau"}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
			`{"choices":[],"usage":{"prompt_tokens":5,"completion_tokens":2}}`,
		},
	))
	service.Admins = []string{"berti@ejemplo.com"}
	user := apptest.User(t, service)
	conversation := apptest.Thread(t, service)
	for _, text := range []string{"hola", "otra vez"} {
		if err := service.Say(t.Context(), conversation.ID, user, text, nil); err != nil {
			t.Fatalf("say: %v", err)
		}
		apptest.Wait(t, service, conversation.ID)
	}

	recorder := httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/admin/telemetry", nil, apptest.Session(t, service, "berti@ejemplo.com")))
	if recorder.Code != http.StatusOK {
		t.Fatalf("contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	var summary metric.Summary
	if err := json.Unmarshal(recorder.Body.Bytes(), &summary); err != nil {
		t.Fatalf("no pude leer la respuesta: %v", err)
	}
	if len(summary.Days) != 1 {
		t.Fatalf("los días quedaron %+v", summary.Days)
	}
	day := summary.Days[0]
	if day.Model != "m1" || day.Turns != 2 || day.Input != 17 || day.Output != 5 {
		t.Fatalf("el día quedó %+v", day)
	}
	if summary.Total.Turns != 2 || summary.Total.Input != 17 || summary.Total.Output != 5 {
		t.Fatalf("el total quedó %+v", summary.Total)
	}
	raw := recorder.Body.String()
	for _, name := range []string{user.ID, conversation.ProjectID, conversation.ID, user.Email} {
		if strings.Contains(raw, name) {
			t.Fatalf("la respuesta lleva %q: %s", name, raw)
		}
	}
}

func TestTelemetryRejectsAWindowOutsideTheYear(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	service.Admins = []string{"berti@ejemplo.com"}
	cookie := apptest.Session(t, service, "berti@ejemplo.com")
	for _, days := range []string{"0", "366", "todos"} {
		recorder := httptest.NewRecorder()
		Get(recorder, apptest.Request(t, "GET", "/api/admin/telemetry?days="+days, nil, cookie))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("days=%s contestó %d", days, recorder.Code)
		}
	}
}

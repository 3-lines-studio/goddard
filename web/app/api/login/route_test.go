package login

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/3-lines-studio/goddard/web/app/apptest"
)

func TestPostHandsTheLinkBackWhenThereIsNoMail(t *testing.T) {
	apptest.Route(t, apptest.Provider(t))
	recorder := httptest.NewRecorder()
	Post(recorder, apptest.Request(t, "POST", "/api/login", map[string]string{"email": "Berti@Ejemplo.com"}, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	var body struct {
		Sent bool   `json:"sent"`
		Link string `json:"link"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("no pude leer la respuesta: %v", err)
	}
	if body.Sent || !strings.Contains(body.Link, "/auth?token=") {
		t.Fatalf("la respuesta quedó %+v", body)
	}
}

func TestPostRejectsWhatItCannot(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	recorder := httptest.NewRecorder()
	Post(recorder, apptest.Request(t, "POST", "/api/login", map[string]string{"email": "no-es-un-mail"}, nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("un mail que no es un mail contestó %d", recorder.Code)
	}

	service.Allowed = []string{"berti@ejemplo.com"}
	recorder = httptest.NewRecorder()
	Post(recorder, apptest.Request(t, "POST", "/api/login", map[string]string{"email": "otro@ejemplo.com"}, nil))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("afuera de la lista contestó %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	Post(recorder, apptest.Request(t, "POST", "/api/login", map[string]string{"email": "berti@ejemplo.com"}, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("el de la lista contestó %d", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	Post(recorder, apptest.Request(t, "POST", "/api/login", map[string]string{"email": "berti@ejemplo.com"}, nil))
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("dos veces seguidas contestó %d", recorder.Code)
	}
}

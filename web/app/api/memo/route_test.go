package memo

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/3-lines-studio/goddard/web/app/apptest"
)

func TestMemoWantsASession(t *testing.T) {
	apptest.Route(t, apptest.Provider(t))
	recorder := httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/memo?project=goddard", nil, nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("sin sesión contestó %d", recorder.Code)
	}
}

func TestMemoIsTheGeneralFactsAndTheOnesOfThatProject(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, "berti@ejemplo.com")
	apptest.Thread(t, service)
	scope, ok := service.Scope(t.Context(), apptest.User(t, service), "goddard")
	if !ok {
		t.Fatal("no pude armar el ámbito del proyecto")
	}
	for _, hecho := range []struct{ key, kind, body string }{
		{"usuario", "identidad", "Es Don Berti."},
		{"goddard/deploy", "estado", "Sale de staging."},
		{"otro/cola", "decision", "No es de este proyecto."},
	} {
		if _, err := service.Memo.Add(t.Context(), scope, hecho.key, hecho.kind, hecho.body); err != nil {
			t.Fatalf("no pude sembrar %q: %v", hecho.key, err)
		}
	}

	recorder := httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/memo?project=goddard", nil, cookie))
	if recorder.Code != http.StatusOK {
		t.Fatalf("contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	var body struct {
		Facts []struct {
			Key     string `json:"key"`
			Kind    string `json:"kind"`
			Body    string `json:"body"`
			Project string `json:"project"`
			Date    int64  `json:"date"`
		} `json:"facts"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("no pude leer la respuesta: %v", err)
	}
	claves := map[string]string{}
	for _, fact := range body.Facts {
		claves[fact.Key] = fact.Kind
		if fact.Date == 0 {
			t.Fatalf("el hecho %q vino sin fecha", fact.Key)
		}
	}
	if len(claves) != 2 || claves["usuario"] != "identidad" || claves["goddard/deploy"] != "estado" {
		t.Fatalf("llegaron %+v", claves)
	}
	if _, ok := claves["otro/cola"]; ok {
		t.Fatal("se coló un hecho de otro proyecto")
	}
}

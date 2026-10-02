package app_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/3-lines-studio/goddard/chat"
	"github.com/3-lines-studio/goddard/heimdall"
	"github.com/3-lines-studio/goddard/web/app"
	"github.com/3-lines-studio/goddard/web/app/apptest"
)

func TestTheModelOfAnOwnerIsTheOneOfTheHouseByDefault(t *testing.T) {
	service := apptest.Service(t, apptest.Provider(t))
	owner := chat.Owner{Kind: chat.OwnerUser, ID: apptest.User(t, service).ID}
	provider, model, err := service.ModelOf(t.Context(), owner)
	if err != nil {
		t.Fatalf("modelo: %v", err)
	}
	if provider != service.Provider || model != service.Model {
		t.Fatalf("el modelo quedó %q y no el de la casa", model)
	}
}

func TestSavingAModelMakesItTheOneOfTheOwner(t *testing.T) {
	service := apptest.Service(t, apptest.Provider(t))
	owner := chat.Owner{Kind: chat.OwnerUser, ID: apptest.User(t, service).ID}
	brought := app.Model{Base: "https://api.ejemplo.com", Name: "mi-modelo", Key: []byte("una-clave")}
	if err := service.SaveModel(t.Context(), owner, brought, "test"); err != nil {
		t.Fatalf("guardar: %v", err)
	}
	own, err := service.OwnModel(t.Context(), owner)
	if err != nil {
		t.Fatalf("leer: %v", err)
	}
	if err := own.Ready(); err != nil {
		t.Fatalf("el modelo no quedó listo: %v", err)
	}
	if own.Base != brought.Base || own.Name != brought.Name || string(own.Key) != string(brought.Key) {
		t.Fatalf("el modelo quedó %+v", own)
	}
	provider, model, err := service.ModelOf(t.Context(), owner)
	if err != nil {
		t.Fatalf("modelo: %v", err)
	}
	if model != brought.Name {
		t.Fatalf("el modelo quedó %q", model)
	}
	if provider == service.Provider {
		t.Fatal("siguió usando el de la casa")
	}
}

func TestAHalfLoadedModelSaysWhichPieceIsMissing(t *testing.T) {
	service := apptest.Service(t, apptest.Provider(t))
	user := apptest.User(t, service)
	owner := chat.Owner{Kind: chat.OwnerUser, ID: user.ID}
	if err := service.Heimdall.Set(t.Context(), heimdall.User(user.ID), app.ModelProject, app.ModelEnv, app.ModelBase, "https://api.ejemplo.com", "test"); err != nil {
		t.Fatalf("base: %v", err)
	}
	_, _, err := service.ModelOf(t.Context(), owner)
	if !errors.Is(err, app.ErrNoModel) {
		t.Fatalf("con medio modelo devolvió %v", err)
	}
	if !strings.Contains(err.Error(), app.ModelName) {
		t.Fatalf("no dijo cuál falta: %v", err)
	}
	if err := service.Heimdall.Set(t.Context(), heimdall.User(user.ID), app.ModelProject, app.ModelEnv, app.ModelName, "mi-modelo", "test"); err != nil {
		t.Fatalf("nombre: %v", err)
	}
	_, _, err = service.ModelOf(t.Context(), owner)
	if !errors.Is(err, app.ErrNoModel) {
		t.Fatalf("sin la clave devolvió %v", err)
	}
	if !strings.Contains(err.Error(), app.ModelKey) {
		t.Fatalf("no dijo cuál falta: %v", err)
	}
}

func TestClearingTheModelGoesBackToTheHouse(t *testing.T) {
	service := apptest.Service(t, apptest.Provider(t))
	owner := chat.Owner{Kind: chat.OwnerUser, ID: apptest.User(t, service).ID}
	if err := service.SaveModel(t.Context(), owner, app.Model{Base: "https://api.ejemplo.com", Name: "mi-modelo", Key: []byte("una-clave")}, "test"); err != nil {
		t.Fatalf("guardar: %v", err)
	}
	if err := service.ClearModel(t.Context(), owner, "test"); err != nil {
		t.Fatalf("sacar: %v", err)
	}
	provider, model, err := service.ModelOf(t.Context(), owner)
	if err != nil {
		t.Fatalf("modelo: %v", err)
	}
	if provider != service.Provider || model != service.Model {
		t.Fatalf("no volvió al de la casa: %q", model)
	}
	if err := service.ClearModel(t.Context(), owner, "test"); err != nil {
		t.Fatalf("sacar dos veces: %v", err)
	}
}

func TestATurnAnswersWithTheModelOfTheOwner(t *testing.T) {
	asked := make(chan map[string]any, 1)
	brought := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("no pude leer el turno: %v", err)
		}
		select {
		case asked <- body:
		default:
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(apptest.SSE(
			`{"choices":[{"delta":{"content":"hola"}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		)))
	}))
	defer brought.Close()

	service := apptest.Service(t, apptest.Provider(t))
	conversation := apptest.Thread(t, service)
	user := apptest.User(t, service)
	owner := chat.Owner{Kind: chat.OwnerUser, ID: user.ID}
	if err := service.SaveModel(t.Context(), owner, app.Model{Base: brought.URL, Name: "mi-modelo", Key: []byte("una-clave")}, "test"); err != nil {
		t.Fatalf("guardar: %v", err)
	}
	if err := service.Say(t.Context(), conversation.ID, user, "hola", nil); err != nil {
		t.Fatalf("say: %v", err)
	}
	apptest.Wait(t, service, conversation.ID)

	select {
	case body := <-asked:
		if body["model"] != "mi-modelo" {
			t.Fatalf("pidió el modelo %v", body["model"])
		}
		if !strings.Contains(fmt.Sprint(body["messages"]), "Modelo: mi-modelo") {
			t.Fatalf("el prompt no dice el modelo propio: %v", body["messages"])
		}
	case <-time.After(5 * time.Second):
		t.Fatal("el turno no salió por el modelo del dueño")
	}
}

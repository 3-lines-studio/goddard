package uploads

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/3-lines-studio/goddard/web/app/apptest"
)

func fileRequest(t *testing.T, conversation, name string, content []byte, cookie *http.Cookie) *http.Request {
	t.Helper()
	body := bytes.Buffer{}
	writer := multipart.NewWriter(&body)
	if conversation != "" {
		if err := writer.WriteField("conversation", conversation); err != nil {
			t.Fatalf("no pude escribir la conversación: %v", err)
		}
	}
	if name != "" {
		part, err := writer.CreateFormFile("file", name)
		if err != nil {
			t.Fatalf("no pude escribir el archivo: %v", err)
		}
		if _, err := part.Write(content); err != nil {
			t.Fatalf("no pude escribir el archivo: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("no pude cerrar el formulario: %v", err)
	}
	request := httptest.NewRequest("POST", "/api/uploads", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if cookie != nil {
		request.AddCookie(cookie)
	}
	return request
}

func TestTheRoutesWantASession(t *testing.T) {
	apptest.Route(t, apptest.Provider(t))
	recorder := httptest.NewRecorder()
	Post(recorder, fileRequest(t, "c", "nota.txt", []byte("hola"), nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("subir sin sesión contestó %d", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/uploads?id=c", nil, nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("bajar sin sesión contestó %d", recorder.Code)
	}
}

func TestAnUploadGoesAndComesBack(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, "berti@ejemplo.com")
	thread := apptest.Thread(t, service)
	content := []byte("hola, soy un archivo")

	recorder := httptest.NewRecorder()
	Post(recorder, fileRequest(t, thread.ID, "nota.txt", content, cookie))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("subir contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	var subido struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Mime string `json:"mime"`
		Size int    `json:"size"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &subido); err != nil {
		t.Fatalf("no pude leer la respuesta: %v", err)
	}
	if subido.Name != "nota.txt" || subido.Size != len(content) || subido.Mime == "" {
		t.Fatalf("la respuesta quedó %+v", subido)
	}

	recorder = httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/uploads?id="+subido.ID, nil, cookie))
	if recorder.Code != http.StatusOK {
		t.Fatalf("bajar contestó %d", recorder.Code)
	}
	if !bytes.Equal(recorder.Body.Bytes(), content) {
		t.Fatalf("bajó %q", recorder.Body.String())
	}
	if got := recorder.Result().Header.Get("Content-Type"); got != subido.Mime {
		t.Fatalf("el tipo quedó %q", got)
	}
}

func TestUploadRejectsWhatItCannot(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, "berti@ejemplo.com")
	thread := apptest.Thread(t, service)

	recorder := httptest.NewRecorder()
	Post(recorder, fileRequest(t, "no-existe", "nota.txt", []byte("hola"), cookie))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("sin conversación contestó %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	Post(recorder, fileRequest(t, thread.ID, "grande.bin", make([]byte, MaxBytes+1), cookie))
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("el archivo grande contestó %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/uploads?id=no-existe", nil, cookie))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("un archivo que no existe contestó %d", recorder.Code)
	}
}

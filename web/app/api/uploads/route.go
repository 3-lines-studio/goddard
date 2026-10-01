package uploads

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"

	"github.com/3-lines-studio/goddard/web/app"
)

// MaxBytes is what an attachment may weigh. It is the limit of the chat
// itself, not of what the model reads: a picture is worth sending, a disk is
// not.
const MaxBytes = 10 << 20

func Post(w http.ResponseWriter, r *http.Request) {
	service := app.Current()
	if service == nil {
		http.Error(w, "el servicio no arrancó", http.StatusServiceUnavailable)
		return
	}
	if _, ok := app.Session(service, w, r); !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxBytes)
	if err := r.ParseMultipartForm(MaxBytes); err != nil {
		http.Error(w, "no pude leer el archivo", http.StatusRequestEntityTooLarge)
		return
	}
	conversation := r.FormValue("conversation")
	if _, ok, err := service.Chat.Conversation(r.Context(), conversation); err != nil || !ok {
		http.Error(w, "esa conversación no existe", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "falta el archivo", http.StatusBadRequest)
		return
	}
	defer file.Close()
	bytes, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	mime := header.Header.Get("Content-Type")
	if mime == "" {
		mime = http.DetectContentType(bytes)
	}
	upload, err := service.Chat.PutUpload(r.Context(), conversation, filepath.Base(header.Filename), mime, bytes)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":   upload.ID,
		"name": upload.Name,
		"mime": upload.Mime,
		"size": len(bytes),
	})
}

func Get(w http.ResponseWriter, r *http.Request) {
	service := app.Current()
	if service == nil {
		http.Error(w, "el servicio no arrancó", http.StatusServiceUnavailable)
		return
	}
	if _, ok := app.Session(service, w, r); !ok {
		return
	}
	upload, ok, err := service.Chat.Upload(r.Context(), r.URL.Query().Get("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "ese archivo no existe", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", upload.Mime)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", upload.Name))
	w.Header().Set("Cache-Control", "private, max-age=86400")
	_, _ = w.Write(upload.Bytes)
}

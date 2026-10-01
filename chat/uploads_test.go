package chat

import (
	"bytes"
	"testing"
)

func TestAnUploadGoesAndComesBackWhole(t *testing.T) {
	store := testStore(t)
	conversation := thread(t, store)
	bytesIn := []byte{0x89, 'P', 'N', 'G', 0x00, 0xff, 0x10}
	stored, err := store.PutUpload(t.Context(), conversation.ID, "foto.png", "image/png", bytesIn)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if stored.ID == "" {
		t.Fatal("el adjunto quedó sin id")
	}
	found, ok, err := store.Upload(t.Context(), stored.ID)
	if err != nil || !ok {
		t.Fatalf("no lo encontré: %v %v", ok, err)
	}
	if found.Name != "foto.png" || found.Mime != "image/png" || !bytes.Equal(found.Bytes, bytesIn) {
		t.Fatalf("volvió %+v con %d bytes", found, len(found.Bytes))
	}
}

func TestTheListDoesNotCarryTheBytes(t *testing.T) {
	store := testStore(t)
	conversation := thread(t, store)
	if _, err := store.PutUpload(t.Context(), conversation.ID, "una.png", "image/png", []byte("una imagen larga")); err != nil {
		t.Fatalf("put: %v", err)
	}
	if _, err := store.PutUpload(t.Context(), conversation.ID, "otra.png", "image/png", []byte("otra imagen")); err != nil {
		t.Fatalf("put: %v", err)
	}
	uploads, err := store.Uploads(t.Context(), conversation.ID)
	if err != nil {
		t.Fatalf("uploads: %v", err)
	}
	if len(uploads) != 2 {
		t.Fatalf("volvieron %d", len(uploads))
	}
	for _, upload := range uploads {
		if upload.Bytes != nil {
			t.Fatalf("la lista trajo %d bytes de %s", len(upload.Bytes), upload.Name)
		}
	}
}

func TestAnUploadIsNotThereForAnotherConversation(t *testing.T) {
	store := testStore(t)
	project, err := store.CreateProject(t.Context(), "goddard", "berti")
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	one := threadIn(t, store, project.ID)
	two := threadIn(t, store, project.ID)
	if _, err := store.PutUpload(t.Context(), one.ID, "una.png", "image/png", []byte("x")); err != nil {
		t.Fatalf("put: %v", err)
	}
	uploads, err := store.Uploads(t.Context(), two.ID)
	if err != nil {
		t.Fatalf("uploads: %v", err)
	}
	if len(uploads) != 0 {
		t.Fatalf("el otro hilo tiene %+v", uploads)
	}
}

func TestAnUploadOfAConversationThatIsNotThereIsRefused(t *testing.T) {
	store := testStore(t)
	if _, err := store.PutUpload(t.Context(), "no-existe", "una.png", "image/png", []byte("x")); err == nil {
		t.Fatal("aceptó un adjunto de una conversación que no existe")
	}
}

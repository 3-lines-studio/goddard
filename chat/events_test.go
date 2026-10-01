package chat

import (
	"encoding/json"
	"sync"
	"testing"
)

func thread(t *testing.T, store *Store) Conversation {
	t.Helper()
	found, err := store.CreateProject(t.Context(), "goddard", Owner{Kind: OwnerUser, ID: "berti"}, "berti")
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	return threadIn(t, store, found.ID)
}

func threadIn(t *testing.T, store *Store, projectID string) Conversation {
	t.Helper()
	conversation, err := store.CreateConversation(t.Context(), projectID, "el hilo", "", "berti")
	if err != nil {
		t.Fatalf("conversation: %v", err)
	}
	return conversation
}

func TestTheLogNumbersItsLines(t *testing.T) {
	store := testStore(t)
	conversation := thread(t, store)
	first, err := store.Append(t.Context(), conversation.ID, []byte(`{"event":"user","text":"hola"}`))
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	second, err := store.Append(t.Context(), conversation.ID, []byte(`{"event":"assistant","text":"qué tal"}`))
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if first.Seq != 1 || second.Seq != 2 {
		t.Fatalf("los números quedaron %d y %d", first.Seq, second.Seq)
	}
	if first.At == 0 {
		t.Fatal("la línea no dice cuándo fue")
	}
}

func TestTheStreamAsksFromWhatItAlreadySaw(t *testing.T) {
	store := testStore(t)
	conversation := thread(t, store)
	for _, text := range []string{"uno", "dos", "tres"} {
		if _, err := store.Append(t.Context(), conversation.ID, []byte(`{"event":"user","text":"`+text+`"}`)); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	events, err := store.Events(t.Context(), conversation.ID, 0)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("volvieron %d", len(events))
	}
	events, err = store.Events(t.Context(), conversation.ID, 1)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(events) != 2 || events[0].Seq != 2 {
		t.Fatalf("desde el 1 volvieron %+v", events)
	}
	var body struct {
		Event string `json:"event"`
		Text  string `json:"text"`
	}
	if err := json.Unmarshal(events[1].Body, &body); err != nil {
		t.Fatalf("no pude leer el cuerpo: %v", err)
	}
	if body.Event != "user" || body.Text != "tres" {
		t.Fatalf("el cuerpo volvió %+v", body)
	}
}

func TestAThreadIsSeparateFromTheOthers(t *testing.T) {
	store := testStore(t)
	project, err := store.CreateProject(t.Context(), "goddard", Owner{Kind: OwnerUser, ID: "berti"}, "berti")
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	one := threadIn(t, store, project.ID)
	two := threadIn(t, store, project.ID)
	if _, err := store.Append(t.Context(), one.ID, []byte(`{"event":"user","text":"de uno"}`)); err != nil {
		t.Fatalf("append: %v", err)
	}
	events, err := store.Events(t.Context(), two.ID, 0)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("el otro hilo tiene %+v", events)
	}
}

func TestAppendingTouchesTheConversation(t *testing.T) {
	store := testStore(t)
	conversation := thread(t, store)
	if _, err := store.Append(t.Context(), conversation.ID, []byte(`{"event":"user","text":"hola"}`)); err != nil {
		t.Fatalf("append: %v", err)
	}
	found, ok, err := store.Conversation(t.Context(), conversation.ID)
	if err != nil || !ok {
		t.Fatalf("no la encontré: %v %v", ok, err)
	}
	if found.UpdatedAt < conversation.UpdatedAt {
		t.Fatalf("quedó en %d, antes %d", found.UpdatedAt, conversation.UpdatedAt)
	}
}

func TestTwoWritersDoNotFightForTheNumber(t *testing.T) {
	store := testStore(t)
	conversation := thread(t, store)
	seen := make([]int64, 20)
	var wait sync.WaitGroup
	for index := range seen {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			event, err := store.Append(t.Context(), conversation.ID, []byte(`{"event":"user","text":"x"}`))
			if err != nil {
				t.Errorf("append %d: %v", index, err)
				return
			}
			seen[index] = event.Seq
		}(index)
	}
	wait.Wait()
	numbers := map[int64]bool{}
	for _, seq := range seen {
		if seq == 0 {
			t.Fatal("una línea se quedó sin número")
		}
		if numbers[seq] {
			t.Fatalf("el número %d se repitió", seq)
		}
		numbers[seq] = true
	}
	if len(numbers) != len(seen) {
		t.Fatalf("quedaron %d números para %d líneas", len(numbers), len(seen))
	}
}

func TestATurnIsClaimedOnce(t *testing.T) {
	store := testStore(t)
	conversation := thread(t, store)
	taken, err := store.Claim(t.Context(), conversation.ID, 900)
	if err != nil || !taken {
		t.Fatalf("el primero no pudo: %v %v", taken, err)
	}
	found, ok, err := store.Conversation(t.Context(), conversation.ID)
	if err != nil || !ok {
		t.Fatalf("no la encontré: %v %v", ok, err)
	}
	if found.ClaimedUntil == 0 {
		t.Fatal("la conversación no dice que está tomada")
	}
	again, err := store.Claim(t.Context(), conversation.ID, 900)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if again {
		t.Fatal("el segundo turno entró igual")
	}
	if err := store.Release(t.Context(), conversation.ID); err != nil {
		t.Fatalf("release: %v", err)
	}
	after, err := store.Claim(t.Context(), conversation.ID, 900)
	if err != nil || !after {
		t.Fatalf("después del release no pudo: %v %v", after, err)
	}
}

func TestAClaimThatExpiredIsFree(t *testing.T) {
	store := testStore(t)
	conversation := thread(t, store)
	if taken, err := store.Claim(t.Context(), conversation.ID, -1); err != nil || !taken {
		t.Fatalf("el primero no pudo: %v %v", taken, err)
	}
	if taken, err := store.Claim(t.Context(), conversation.ID, 900); err != nil || !taken {
		t.Fatalf("el vencido siguió tomado: %v %v", taken, err)
	}
}

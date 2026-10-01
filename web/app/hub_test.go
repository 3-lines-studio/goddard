package app

import (
	"testing"
	"time"
)

func TestTheHubReachesWhoeverIsListening(t *testing.T) {
	hub := newHub()
	one, leavingOne := hub.listen("c1")
	defer leavingOne()
	two, leavingTwo := hub.listen("c1")
	defer leavingTwo()
	other, leavingOther := hub.listen("c2")
	defer leavingOther()
	hub.tell("c1", []byte("hola"))
	for index, channel := range []chan []byte{one, two} {
		select {
		case body := <-channel:
			if string(body) != "hola" {
				t.Fatalf("el %d recibió %q", index, body)
			}
		case <-time.After(time.Second):
			t.Fatalf("el %d no recibió nada", index)
		}
	}
	select {
	case body := <-other:
		t.Fatalf("el otro hilo vio %q", body)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestTheHubDoesNotWaitForWhoeverLeft(t *testing.T) {
	hub := newHub()
	channel, leaving := hub.listen("c1")
	leaving()
	for index := 0; index < 100; index++ {
		hub.tell("c1", []byte("nadie lee"))
	}
	if len(channel) > cap(channel) {
		t.Fatalf("el canal quedó con %d", len(channel))
	}
	if _, ok := hub.byID["c1"]; ok {
		t.Fatal("el hilo quedó en el mapa después de irse el que miraba")
	}
}

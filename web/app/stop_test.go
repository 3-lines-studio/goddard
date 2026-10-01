package app

import (
	"context"
	"testing"
)

func TestAStopReachesTheTurnThatIsRunning(t *testing.T) {
	stops := newStops()
	turn, cancel := context.WithCancel(context.Background())
	stops.add("c1", cancel)
	if !stops.stop("c1") {
		t.Fatal("no encontró el turno que corría")
	}
	select {
	case <-turn.Done():
	default:
		t.Fatal("el turno sigue vivo después del stop")
	}
	if stops.stop("c2") {
		t.Fatal("encontró un turno en una conversación donde no hay ninguno")
	}
	stops.drop("c1")
	if stops.stop("c1") {
		t.Fatal("el turno sigue ahí después de terminar")
	}
}

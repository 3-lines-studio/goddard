package schedule

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type aviso struct {
	destinos []string
	textos   []string
	err      error
}

func (a *aviso) Notify(_ context.Context, target, text string) error {
	if a.err != nil {
		return a.err
	}
	a.destinos = append(a.destinos, target)
	a.textos = append(a.textos, text)
	return nil
}

func runnerDe(texto string, err error) (Runner, *int) {
	llamadas := 0
	run := func(_ context.Context, _ Task) (string, error) {
		llamadas++
		return texto, err
	}
	return run, &llamadas
}

func TestOnceCorreLoQueEstaDue(t *testing.T) {
	store, _ := testStore(t)
	ctx := t.Context()
	if err := store.Add(ctx, unaTarea("daily")); err != nil {
		t.Fatal(err)
	}
	run, llamadas := runnerDe("listo, sin novedades", nil)
	service := NewService(store, run, 0)

	if err := service.Once(ctx, reloj()); err != nil {
		t.Fatal(err)
	}
	if *llamadas != 1 {
		t.Fatalf("corrió %d veces", *llamadas)
	}
	entries, err := store.List(ctx, deBerti(), "goddard")
	if err != nil {
		t.Fatal(err)
	}
	runs := entries[0].Runs
	if len(runs) != 1 {
		t.Fatalf("quedaron %d corridas", len(runs))
	}
	if !runs[0].OK || runs[0].Text != "listo, sin novedades" {
		t.Fatalf("la corrida quedó %+v", runs[0])
	}
	if runs[0].Date != "2026-09-14" {
		t.Fatalf("la corrida quedó fechada %q", runs[0].Date)
	}
}

func TestOnceNoCorreLoQueNoEstaDue(t *testing.T) {
	store, _ := testStore(t)
	ctx := t.Context()
	tarde := unaTarea("mas-tarde")
	tarde.At = "23:00"
	if err := store.Add(ctx, tarde); err != nil {
		t.Fatal(err)
	}
	run, llamadas := runnerDe("no debería", nil)
	service := NewService(store, run, 0)

	if err := service.Once(ctx, reloj()); err != nil {
		t.Fatal(err)
	}
	if *llamadas != 0 {
		t.Fatalf("corrió %d veces una tarea que no le tocaba", *llamadas)
	}
}

func TestUnaCorridaQueFallaQuedaEnElLog(t *testing.T) {
	store, _ := testStore(t)
	ctx := t.Context()
	if err := store.Add(ctx, unaTarea("daily")); err != nil {
		t.Fatal(err)
	}
	run, _ := runnerDe("", errors.New("se cayó el modelo"))
	service := NewService(store, run, 0)

	if err := service.Once(ctx, reloj()); err != nil {
		t.Fatal(err)
	}
	entries, err := store.List(ctx, deBerti(), "goddard")
	if err != nil {
		t.Fatal(err)
	}
	runs := entries[0].Runs
	if len(runs) != 1 || runs[0].OK || runs[0].Text != "se cayó el modelo" {
		t.Fatalf("la corrida quedó %+v", runs)
	}
}

func TestRunNowNoEsperaAlTick(t *testing.T) {
	store, _ := testStore(t)
	ctx := t.Context()
	tarde := unaTarea("mas-tarde")
	tarde.At = "23:00"
	if err := store.Add(ctx, tarde); err != nil {
		t.Fatal(err)
	}
	run, llamadas := runnerDe("a pedido", nil)
	service := NewService(store, run, 0)

	if _, err := service.RunNow(ctx, deBerti(), "goddard", "mas-tarde"); err != nil {
		t.Fatal(err)
	}
	if *llamadas != 1 {
		t.Fatalf("corrió %d veces", *llamadas)
	}
	entries, err := store.List(ctx, deBerti(), "goddard")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries[0].Runs) != 1 || entries[0].Runs[0].Text != "a pedido" {
		t.Fatalf("la corrida quedó %+v", entries[0].Runs)
	}

	if _, err := service.RunNow(ctx, deBerti(), "goddard", "no-existe"); err == nil {
		t.Fatal("correr lo que no existe no falló")
	}
}

func TestElAvisoVaAlTarget(t *testing.T) {
	store, _ := testStore(t)
	ctx := t.Context()
	task := unaTarea("daily")
	task.Target = "123456789"
	if err := store.Add(ctx, task); err != nil {
		t.Fatal(err)
	}
	run, _ := runnerDe("todo bien", nil)
	destinos := &aviso{}
	service := NewService(store, run, 0).WithNotifier(destinos)

	if err := service.Once(ctx, reloj()); err != nil {
		t.Fatal(err)
	}
	if len(destinos.destinos) != 1 || destinos.destinos[0] != "123456789" {
		t.Fatalf("avisó a %v", destinos.destinos)
	}
	if destinos.textos[0] != "todo bien" {
		t.Fatalf("avisó %q", destinos.textos[0])
	}
}

func TestUnTargetQueNoSaleNoSeLlevaLaCorrida(t *testing.T) {
	store, _ := testStore(t)
	ctx := t.Context()
	task := unaTarea("daily")
	task.Target = "123456789"
	if err := store.Add(ctx, task); err != nil {
		t.Fatal(err)
	}
	run, _ := runnerDe("todo bien", nil)
	service := NewService(store, run, 0).WithNotifier(&aviso{err: errors.New("no conozco el chat")})

	if err := service.Once(ctx, reloj()); err != nil {
		t.Fatal(err)
	}
	entries, err := store.List(ctx, deBerti(), "goddard")
	if err != nil {
		t.Fatal(err)
	}
	runs := entries[0].Runs
	if len(runs) != 1 || !runs[0].OK {
		t.Fatalf("la corrida quedó %+v", runs)
	}
	if !strings.Contains(runs[0].Text, "⚠️ no salió a 123456789") {
		t.Fatalf("el historial no dice por qué no salió: %q", runs[0].Text)
	}
	if !strings.HasPrefix(runs[0].Text, "todo bien") {
		t.Fatalf("se perdió la respuesta: %q", runs[0].Text)
	}
}

func TestSinRespuestaNoHayNadaQueAvisar(t *testing.T) {
	store, _ := testStore(t)
	ctx := t.Context()
	task := unaTarea("vigia")
	task.Target = "123456789"
	task.Silent = true
	if err := store.Add(ctx, task); err != nil {
		t.Fatal(err)
	}
	run, _ := runnerDe("", nil)
	destinos := &aviso{}
	service := NewService(store, run, 0).WithNotifier(destinos)

	if err := service.Once(ctx, reloj()); err != nil {
		t.Fatal(err)
	}
	if len(destinos.destinos) != 0 {
		t.Fatalf("avisó al vacío: %v", destinos.textos)
	}
}

func TestSinConQueAvisarElHistorialLoSabe(t *testing.T) {
	store, _ := testStore(t)
	ctx := t.Context()
	task := unaTarea("daily")
	task.Target = "123456789"
	if err := store.Add(ctx, task); err != nil {
		t.Fatal(err)
	}
	run, _ := runnerDe("todo bien", nil)

	if err := NewService(store, run, 0).Once(ctx, reloj()); err != nil {
		t.Fatal(err)
	}
	entries, err := store.List(ctx, deBerti(), "goddard")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(entries[0].Runs[0].Text, "no hay con qué avisar") {
		t.Fatalf("el historial quedó %q", entries[0].Runs[0].Text)
	}
}

func TestDosServiciosNoRepitenLaCorrida(t *testing.T) {
	store, db := testStore(t)
	ctx := t.Context()
	for _, name := range []string{"uno", "dos"} {
		if err := store.Add(ctx, unaTarea(name)); err != nil {
			t.Fatal(err)
		}
	}
	runA, llamadasA := runnerDe("a", nil)
	runB, llamadasB := runnerDe("b", nil)
	a := NewService(store, runA, 0)
	b := NewService(NewPgStore(db), runB, 0)

	if err := a.Once(ctx, reloj()); err != nil {
		t.Fatal(err)
	}
	if err := b.Once(ctx, reloj()); err != nil {
		t.Fatal(err)
	}
	if *llamadasA+*llamadasB != 2 {
		t.Fatalf("corrieron %d y %d veces", *llamadasA, *llamadasB)
	}
}

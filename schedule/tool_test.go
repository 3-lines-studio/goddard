package schedule

import (
	"strings"
	"testing"
)

// TestLaToolAgendaYLee la corre como la corre el harness: JSON crudo adentro,
// texto afuera.
func TestLaToolAgendaYLee(t *testing.T) {
	store, _ := testStore(t)
	tool := Tool(store, "berti", "goddard")

	got := tool.Run(`{"action":"add","name":"recordatorio-tests","at":"15:00","prompt":"Avisá que corra los tests"}`, nil)
	if got.Text != "guardada: recordatorio-tests · todos los días 15:00" {
		t.Fatalf("add quedó %q", got.Text)
	}

	got = tool.Run(`{"action":"list"}`, nil)
	if got.Text != "recordatorio-tests · todos los días 15:00\n  todavía no corrió" {
		t.Fatalf("list quedó %q", got.Text)
	}

	got = tool.Run(`{"action":"show","name":"recordatorio-tests"}`, nil)
	want := "recordatorio-tests · todos los días 15:00\nprompt: Avisá que corra los tests\ntodavía no corrió"
	if got.Text != want {
		t.Fatalf("show quedó %q", got.Text)
	}

	got = tool.Run(`{"action":"pause","name":"recordatorio-tests"}`, nil)
	if got.Text != "pausada: recordatorio-tests" {
		t.Fatalf("pause quedó %q", got.Text)
	}
	got = tool.Run(`{"action":"list"}`, nil)
	if !strings.Contains(got.Text, " · pausada") {
		t.Fatalf("list no dice que está pausada: %q", got.Text)
	}
	got = tool.Run(`{"action":"resume","name":"recordatorio-tests"}`, nil)
	if got.Text != "en marcha: recordatorio-tests" {
		t.Fatalf("el despausado quedó %q", got.Text)
	}

	got = tool.Run(`{"action":"remove","name":"recordatorio-tests"}`, nil)
	if got.Text != "borrada: recordatorio-tests" {
		t.Fatalf("remove quedó %q", got.Text)
	}
	got = tool.Run(`{"action":"list"}`, nil)
	if got.Text != "no hay tareas" {
		t.Fatalf("la agenda vacía quedó %q", got.Text)
	}
}

func TestLaToolMuestraDeQuienEsLaAgenda(t *testing.T) {
	store, _ := testStore(t)
	if err := store.Add(t.Context(), unaTarea("de-berti")); err != nil {
		t.Fatal(err)
	}

	got := Tool(store, "ana", "goddard").Run(`{"action":"list"}`, nil)
	if got.Text != "no hay tareas" {
		t.Fatalf("ana vio la agenda de otro: %q", got.Text)
	}
}

func TestLaToolMuestraLaUltimaCorrida(t *testing.T) {
	store, _ := testStore(t)
	ctx := t.Context()
	if err := store.Add(ctx, unaTarea("daily")); err != nil {
		t.Fatal(err)
	}
	run, _ := runnerDe("listo, sin novedades", nil)
	if err := NewService(store, run, 0).Once(ctx, reloj()); err != nil {
		t.Fatal(err)
	}

	got := Tool(store, "berti", "goddard").Run(`{"action":"list"}`, nil)
	want := "daily · todos los días 09:00\n  última 2026-09-14 · ok · listo, sin novedades"
	if got.Text != want {
		t.Fatalf("list quedó %q", got.Text)
	}

	got = Tool(store, "berti", "goddard").Run(`{"action":"show","name":"daily"}`, nil)
	if !strings.HasPrefix(got.Text, "daily · todos los días 09:00\nprompt: hacé algo\ncorridas:\n 2026-09-14 · ok · ") {
		t.Fatalf("show quedó %q", got.Text)
	}
}

func TestElListNoSeLlevaLaAgendaPuesta(t *testing.T) {
	store, _ := testStore(t)
	ctx := t.Context()
	if err := store.Add(ctx, unaTarea("daily")); err != nil {
		t.Fatal(err)
	}
	largo := strings.Repeat("a", 500)
	run, _ := runnerDe(largo, nil)
	if err := NewService(store, run, 0).Once(ctx, reloj()); err != nil {
		t.Fatal(err)
	}

	got := Tool(store, "berti", "goddard").Run(`{"action":"list"}`, nil)
	linea := strings.Split(got.Text, "\n")[1]
	if !strings.HasSuffix(linea, "…") {
		t.Fatalf("la corrida larga quedó sin cortar: %q", linea)
	}
	if len([]rune(linea)) > 230 {
		t.Fatalf("la línea quedó de %d runas", len([]rune(linea)))
	}
}

func TestLaToolDiceQueSalióMal(t *testing.T) {
	store, _ := testStore(t)
	tool := Tool(store, "berti", "goddard")
	for _, raw := range []string{
		`{"action":"nope"}`,
		`{"action":"show"}`,
		`{"action":"show","name":"no-existe"}`,
		`{"action":"pause"}`,
		`{"action":"resume"}`,
		`{"action":"remove"}`,
		`{"action":"add","name":"Recordatorio","at":"09:00","prompt":"x"}`,
		`{"action":"add","name":"sin-horario","prompt":"x"}`,
		`{"action":"add","name":"sin-prompt","at":"09:00"}`,
		`{"action":"add","name":"at-roto","at":"25:00","prompt":"x"}`,
	} {
		got := tool.Run(raw, nil)
		if !strings.HasPrefix(got.Text, "error: ") {
			t.Fatalf("%s quedó %q", raw, got.Text)
		}
	}
}

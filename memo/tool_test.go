package memo

import (
	"strings"
	"testing"
)

// TestLaToolEscribeYLee la corre como la corre el harness: JSON crudo adentro,
// texto afuera.
func TestLaToolEscribeYLee(t *testing.T) {
	store, _ := testStore(t)
	tool := Tool(store)

	got := tool.Run(`{"action":"add","key":"jimmy/telemetria","kind":"medicion","text":"un número"}`, nil)
	if !strings.HasPrefix(got.Text, "jimmy/telemetria · medicion · ") {
		t.Fatalf("add quedó %q", got.Text)
	}
	if got.Text[len(got.Text)-len("(nuevo)"):] != "(nuevo)" {
		t.Fatalf("add quedó %q", got.Text)
	}

	got = tool.Run(`{"action":"show","key":"jimmy/telemetria"}`, nil)
	want := "## jimmy/telemetria · medicion · " + date(today()) + "\n\nun número"
	if got.Text != want {
		t.Fatalf("show quedó %q", got.Text)
	}

	got = tool.Run(`{"action":"list"}`, nil)
	if got.Text != "jimmy\n  jimmy/telemetria · medicion · "+date(today()) {
		t.Fatalf("list quedó %q", got.Text)
	}
}

func TestLaToolDiceQueSalióMal(t *testing.T) {
	store, _ := testStore(t)
	tool := Tool(store)
	for _, raw := range []string{
		`{"action":"nope"}`,
		`{"action":"show"}`,
		`{"action":"show","key":"no-existe"}`,
		`{"action":"add","key":"MaYus","kind":"estado","text":"x"}`,
		`{"action":"add","key":"jimmy/x","kind":"inventado","text":"x"}`,
		`{"action":"add","key":"jimmy/x","kind":"estado"}`,
	} {
		got := tool.Run(raw, nil)
		if !strings.HasPrefix(got.Text, "error: ") {
			t.Fatalf("%s quedó %q", raw, got.Text)
		}
	}
}

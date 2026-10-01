package schedule

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestParidadConElRust replays testdata/paridad-rust.txt, what the Rust
// schedule.rs answered for the same cases: the harness compiles that file with
// stubs for the agent and the transport and prints one JSON value per case
// (`projects/agenda-harness`, in the workspace). A case of the dump without a
// counterpart fails the test, and so does a counterpart that is not in the
// dump.
func TestParidadConElRust(t *testing.T) {
	dump := readTestdata(t, "paridad-rust.txt")
	casos := map[string]func() any{
		"hhmm_ok":              func() any { return punto(HHMM("9:5")) },
		"hhmm_espacios":        func() any { return punto(HHMM(" 07:30 ")) },
		"hhmm_hora_invalida":   func() any { return punto(HHMM("24:00")) },
		"hhmm_minuto_invalido": func() any { return punto(HHMM("9:60")) },
		"hhmm_basura":          func() any { return punto(HHMM("nueve")) },
		"hhmm_sin_minutos":     func() any { return punto(HHMM("9")) },
		"period_horas":         func() any { return punto(Period("6h")) },
		"period_minutos":       func() any { return punto(Period("30m")) },
		"period_dias":          func() any { return punto(Period("2d")) },
		"period_segundos":      func() any { return punto(Period("90s")) },
		"period_cero":          func() any { return punto(Period("0m")) },
		"period_espacios":      func() any { return punto(Period(" 15m ")) },
		"period_compuesto":     func() any { return punto(Period("1h30m")) },
		"period_negativo":      func() any { return punto(Period("-5m")) },
		"period_basura":        func() any { return punto(Period("cada rato")) },
		"civil_epoch":          func() any { return civil(0) },
		"civil_bisiesto":       func() any { return civil(11_016) },
		"civil_2024":           func() any { return civil(19_723) },
		"local_utc":            func() any { return local(0, 0) },
		"local_menos_tres":     func() any { return local(0, -3) },
		"local_mas_cinco":      func() any { return local(1_752_000_000, 5) },

		"due_when_antes":       func() any { return due(una("2026-09-14T15:00", "", ""), ninguna, 0, "2026-09-14", "14:59") },
		"due_when_justo":       func() any { return due(una("2026-09-14T15:00", "", ""), ninguna, 0, "2026-09-14", "15:00") },
		"due_when_con_espacio": func() any { return due(una("2026-09-14 15:00", "", ""), ninguna, 0, "2026-09-14", "15:00") },
		"due_when_ya_corrio": func() any {
			return due(una("2026-09-14T15:00", "", ""), corrida(0, "2026-09-14"), 0, "2026-09-15", "10:00")
		},
		"due_at_nuevo_dia": func() any {
			return due(una("", "09:00", ""), corrida(0, "2026-09-14"), 0, "2026-09-15", "09:00")
		},
		"due_at_mismo_dia": func() any {
			return due(una("", "09:00", ""), corrida(0, "2026-09-14"), 0, "2026-09-14", "10:00")
		},
		"due_at_antes_de_hora": func() any { return due(una("", "09:00", ""), ninguna, 0, "2026-09-15", "08:59") },
		"due_at_invalido":      func() any { return due(una("", "25:00", ""), ninguna, 0, "2026-09-15", "23:00") },
		"due_every_sin_corridas": func() any {
			return due(una("", "", "6h"), ninguna, 1_000, "2026-09-14", "00:00")
		},
		"due_every_medio": func() any {
			return due(una("", "", "6h"), corrida(1_000, "2026-09-14"), 1_000+Hour, "2026-09-14", "00:00")
		},
		"due_every_completo": func() any {
			return due(una("", "", "6h"), corrida(1_000, "2026-09-14"), 1_000+6*Hour, "2026-09-14", "00:00")
		},
		"due_sin_horario": func() any { return due(una("", "", ""), ninguna, 0, "2026-09-14", "10:00") },
		"due_when_gana_sobre_at": func() any {
			return due(una("2026-09-14T15:00", "23:00", ""), ninguna, 0, "2026-09-14", "16:00")
		},
	}

	for name, expected := range dump {
		obtener, ok := casos[name]
		if !ok {
			t.Errorf("el caso %q del dump no tiene contraparte en el port", name)
			continue
		}
		got, err := json.Marshal(obtener())
		if err != nil {
			t.Fatalf("caso %s: %v", name, err)
		}
		if string(got) != expected {
			t.Errorf("caso %s: quedó %s, el Rust dio %s", name, got, expected)
		}
	}
	for name := range casos {
		if _, ok := dump[name]; !ok {
			t.Errorf("el caso %q no está en el dump", name)
		}
	}
}

var ninguna []Run

func punto[T any](value T, ok bool) *T {
	if !ok {
		return nil
	}
	return &value
}

func civil(days int64) [3]int64 {
	year, month, day := CivilFromDays(days)
	return [3]int64{year, month, day}
}

func local(now, offset int64) [2]string {
	date, clock := LocalParts(now, offset)
	return [2]string{date, clock}
}

func una(when, at, every string) Task {
	return Task{When: when, At: at, Every: every, Prompt: "p"}
}

func corrida(ts int64, date string) []Run {
	return []Run{{TS: ts, Date: date, MS: 10, OK: true, Text: "ok"}}
}

func due(task Task, runs []Run, now int64, date, time string) bool {
	return Due(task, runs, now, date, time)
}

func readTestdata(t *testing.T, name string) map[string]string {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("no pude leer %s: %v", name, err)
	}
	dump := map[string]string{}
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		caso, value, found := strings.Cut(line, "\t")
		if !found {
			t.Fatalf("línea sin tabulador en %s: %q", name, line)
		}
		dump[caso] = value
	}
	return dump
}

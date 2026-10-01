package memo

import (
	"os"
	"sort"
	"strings"
	"testing"
	"time"
)

// TestParidadConElRust replays testdata/paridad-rust.txt, the output jimmy's
// binary gave for the same facts. The cases the port does not have are named
// here, so a case that shows up in the dump without a counterpart fails the
// test instead of going unnoticed.
func TestParidadConElRust(t *testing.T) {
	dump := readTestdata(t, "paridad-rust.txt")
	casos := map[string]func(t *testing.T) string{
		"render_tres":          func(t *testing.T) string { return Render(general(t), jimmy(t)) },
		"render_sin_cola":      func(t *testing.T) string { return Render(general(t), soloVieja(t)) },
		"render_otro_proyecto": func(t *testing.T) string { return Render(general(t), nil) },
		"render_sin_proyecto":  func(t *testing.T) string { return Render(general(t), nil) },
		"render_vacio":         func(t *testing.T) string { return Render(nil, nil) },
		"list":                 func(t *testing.T) string { return List(todo(t)) },
		"show_vivo":            func(t *testing.T) string { return Show(nueva(t)) },
		"show_transversal":     func(t *testing.T) string { return Show(usuario(t)) },
		"add_clave_invalida":   func(t *testing.T) string { return validateErr(t, "MaYus", "estado", "x") },
		"add_tipo_desconocido": func(t *testing.T) string { return validateErr(t, "jimmy/x", "inventado", "x") },
		"add_vacio":            func(t *testing.T) string { return validateErr(t, "jimmy/x", "estado", "") },
	}
	sinContraparte := map[string]string{
		"show_no_existe":           "el mensaje lo arma pgstore.go sobre una base vacía (pgstore_test.go)",
		"show_borrado":             "el nivel 2 lo guarda el store (pgstore_test.go)",
		"add_proyecto_desconocido": "en goddard no hay catálogo de proyectos que validar",
		"sync":                     "no hay sync: cada Add sabe qué hizo, y lo devuelve (pgstore_test.go)",
		"sync_sin_cambios":         "no hay sync: un Add sin cambios devuelve Unchanged (pgstore_test.go)",
	}
	divergencias := map[string]func(t *testing.T, expected string) string{
		"render_tres":          adaptarCola,
		"list":                 adaptarLista,
		"add_clave_invalida":   adaptarError,
		"add_tipo_desconocido": adaptarError,
		"add_vacio":            adaptarError,
	}

	for name, expected := range dump {
		obtener, ok := casos[name]
		if !ok {
			if _, declared := sinContraparte[name]; !declared {
				t.Errorf("el caso %q del dump no tiene contraparte en el port", name)
			}
			continue
		}
		if adapt, ok := divergencias[name]; ok {
			expected = adapt(t, expected)
		}
		if got := obtener(t); got != expected {
			t.Errorf("caso %s:\nquedó %q\nel Rust dio %q", name, got, expected)
		}
	}
	for name := range casos {
		if _, ok := dump[name]; !ok {
			t.Errorf("el caso %q no está en el dump", name)
		}
	}
	for name := range sinContraparte {
		if _, ok := dump[name]; !ok {
			t.Errorf("el caso %q sin contraparte no está en el dump", name)
		}
	}
}

const colaRust = "[en `notes/projects/jimmy.md` y afuera del prompt: jimmy/vieja]"

// adaptarCola declara la única diferencia del render: en jimmy el aviso apunta
// al archivo del proyecto, y acá no hay archivo que apuntar.
func adaptarCola(t *testing.T, expected string) string {
	t.Helper()
	if !strings.Contains(expected, colaRust) {
		t.Fatalf("el dump ya no trae la cola del Rust: %q", expected)
	}
	return strings.Replace(expected, colaRust, "[afuera del prompt: jimmy/vieja]", 1)
}

// adaptarLista declara las dos diferencias de `list`: los encabezados eran las
// rutas de los archivos y acá son los ámbitos de la memoria, y el orden dentro
// de un ámbito era el del archivo, que en goddard es la clave.
func adaptarLista(t *testing.T, expected string) string {
	t.Helper()
	for _, path := range []string{"notes/memory/entorno.md", "notes/memory/usuario.md", "notes/projects/jimmy.md"} {
		if !strings.Contains(expected, path) {
			t.Fatalf("el dump ya no trae %q: %q", path, expected)
		}
	}
	expected = strings.Replace(expected, "notes/memory/entorno.md\n", "general\n", 1)
	expected = strings.Replace(expected, "notes/memory/usuario.md\n", "", 1)
	expected = strings.Replace(expected, "notes/projects/jimmy.md\n", "jimmy\n", 1)

	lines := strings.Split(expected, "\n")
	out := []string{}
	for index := 0; index < len(lines); {
		head := lines[index]
		index++
		block := []string{}
		for index < len(lines) && strings.HasPrefix(lines[index], "  ") {
			block = append(block, lines[index])
			index++
		}
		sort.Strings(block)
		out = append(out, head)
		out = append(out, block...)
	}
	return strings.Join(out, "\n")
}

// adaptarError declara que el dump trae el envoltorio del comando de jimmy y
// acá el error es el del paquete.
func adaptarError(t *testing.T, expected string) string {
	t.Helper()
	const prefix = "error: jimmy memo: "
	if !strings.HasPrefix(expected, prefix) {
		t.Fatalf("el dump ya no trae el prefijo del CLI: %q", expected)
	}
	return strings.TrimPrefix(expected, prefix)
}

func validateErr(t *testing.T, key, kind, body string) string {
	t.Helper()
	err := Validate(key, kind, body)
	if err == nil {
		t.Fatalf("esperaba un error para %q, %q, %q", key, kind, body)
	}
	return err.Error()
}

func readTestdata(t *testing.T, name string) map[string]string {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("no pude leer %s: %v", name, err)
	}
	dump := map[string]string{}
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		caso, escaped, found := strings.Cut(line, "\t")
		if !found {
			t.Fatalf("línea sin tabulador en %s: %q", name, line)
		}
		dump[caso] = unescape(escaped)
	}
	return dump
}

func unescape(text string) string {
	var out strings.Builder
	for i := 0; i < len(text); i++ {
		if text[i] != '\\' || i+1 >= len(text) {
			out.WriteByte(text[i])
			continue
		}
		i++
		switch text[i] {
		case 'n':
			out.WriteByte('\n')
		case 't':
			out.WriteByte('\t')
		case 'r':
			out.WriteByte('\r')
		case '\\':
			out.WriteByte('\\')
		default:
			out.WriteByte('\\')
			out.WriteByte(text[i])
		}
	}
	return out.String()
}

func day(t *testing.T, text string) int64 {
	t.Helper()
	when, err := time.Parse("2006-01-02", text)
	if err != nil {
		t.Fatalf("no pude leer la fecha %s: %v", text, err)
	}
	return when.Unix() / 86400
}

func entorno(t *testing.T) Fact {
	return Fact{Key: "entorno", Kind: "plataforma", Body: "linux", Date: day(t, "2026-09-27")}
}

func usuario(t *testing.T) Fact {
	return Fact{Key: "usuario", Kind: "identidad", Body: "vive en caba", Date: day(t, "2026-09-30")}
}

func vieja(t *testing.T) Fact {
	return Fact{Project: "jimmy", Key: "jimmy/vieja", Kind: "estado", Body: "vieja", Date: day(t, "2026-08-01")}
}

func media(t *testing.T) Fact {
	return Fact{Project: "jimmy", Key: "jimmy/media", Kind: "decision", Body: "media", Date: day(t, "2026-09-20")}
}

func nueva(t *testing.T) Fact {
	return Fact{Project: "jimmy", Key: "jimmy/nueva", Kind: "bugfix", Body: "nueva", Date: day(t, "2026-09-29")}
}

func general(t *testing.T) []Fact {
	return []Fact{entorno(t), usuario(t)}
}

func jimmy(t *testing.T) []Fact {
	return []Fact{vieja(t), media(t), nueva(t)}
}

func soloVieja(t *testing.T) []Fact {
	return []Fact{vieja(t)}
}

func todo(t *testing.T) []Fact {
	return append(general(t), jimmy(t)...)
}

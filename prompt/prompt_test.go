package prompt

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func fragmentIn(t *testing.T, dir, name, text string) {
	t.Helper()
	path := filepath.Join(dir, name+".md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// dir arma un fs.FS con los fragmentos que se le pasan, con el nombre completo
// (`es-AR/identidad` y no `identidad`).
func dir(t *testing.T, files map[string]string) fs.FS {
	t.Helper()
	root := t.TempDir()
	for name, text := range files {
		fragmentIn(t, root, name, text)
	}
	return os.DirFS(root)
}

func assemble(t *testing.T, language, spec string, dirs []fs.FS, vars ...Var) string {
	t.Helper()
	out, err := Assemble(language, spec, dirs, vars)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	return out
}

func TestJoinsTheFragmentsInSpecOrder(t *testing.T) {
	root := dir(t, map[string]string{"es-AR/uno": "primero\n", "es-AR/dos": "segundo\n"})
	if got := assemble(t, "es-AR", "uno, dos", []fs.FS{root}); got != "primero\n\nsegundo" {
		t.Fatalf("quedó %q", got)
	}
}

func TestAnEarlierDirWins(t *testing.T) {
	over := dir(t, map[string]string{"es-AR/identidad": "mía"})
	under := dir(t, map[string]string{"es-AR/identidad": "de fábrica"})
	if got := assemble(t, "es-AR", "identidad", []fs.FS{over, under}); got != "mía" {
		t.Fatalf("quedó %q", got)
	}
}

func TestAnEmptySpecAssemblesNothing(t *testing.T) {
	if got := assemble(t, "es-AR", "  ", nil); got != "" {
		t.Fatalf("quedó %q", got)
	}
}

func TestAMissingFragmentIsAnError(t *testing.T) {
	root := dir(t, map[string]string{"es-AR/uno": "primero"})
	if _, err := Assemble("es-AR", "no-existe", []fs.FS{root}, nil); err == nil {
		t.Fatal("no se quejó")
	}
}

func TestAnEmptyFragmentFileIsIgnored(t *testing.T) {
	empty := dir(t, map[string]string{"es-AR/identidad": "   \n"})
	under := dir(t, map[string]string{"es-AR/identidad": "de fábrica"})
	if got := assemble(t, "es-AR", "identidad", []fs.FS{empty, under}); got != "de fábrica" {
		t.Fatalf("quedó %q", got)
	}
}

func TestFillsTheVariables(t *testing.T) {
	root := dir(t, map[string]string{
		"es-AR/identidad": "Sos {{asistente}}, el asistente de {{usuario}}.\n",
	})
	got := assemble(t, "es-AR", "identidad", []fs.FS{root}, Var{"usuario", "Ana"}, Var{"asistente", "Jimmy"})
	if got != "Sos Jimmy, el asistente de Ana." {
		t.Fatalf("quedó %q", got)
	}
}

func TestLeavesSingleBracesAlone(t *testing.T) {
	root := dir(t, map[string]string{"es-AR/herramientas": "`[{\"action\":\"goto\"}]`\n"})
	if got := assemble(t, "es-AR", "herramientas", []fs.FS{root}); got != "`[{\"action\":\"goto\"}]`" {
		t.Fatalf("quedó %q", got)
	}
}

func TestAMissingVariableIsAnError(t *testing.T) {
	root := dir(t, map[string]string{"es-AR/identidad": "Hola {{usuario}}"})
	_, err := Assemble("es-AR", "identidad", []fs.FS{root}, nil)
	if err == nil || !strings.Contains(err.Error(), "usuario") {
		t.Fatalf("dijo %v", err)
	}
}

func TestAnUnclosedPlaceholderIsAnError(t *testing.T) {
	root := dir(t, map[string]string{"es-AR/identidad": "Hola {{usuario"})
	if _, err := Assemble("es-AR", "identidad", []fs.FS{root}, []Var{{"usuario", "Ana"}}); err == nil {
		t.Fatal("no se quejó")
	}
}

func TestAnEmptyLanguageAsksForTheDefault(t *testing.T) {
	root := dir(t, map[string]string{"es-AR/identidad": "hola", "en/identidad": "hello"})
	if got := assemble(t, "", "identidad", []fs.FS{root}); got != "hola" {
		t.Fatalf("quedó %q", got)
	}
}

func TestTheLanguageAsksForItsOwnFragments(t *testing.T) {
	root := dir(t, map[string]string{"es-AR/identidad": "hola", "en/identidad": "hello\n"})
	if got := assemble(t, "en", "identidad", []fs.FS{root}); got != "hello" {
		t.Fatalf("en quedó %q", got)
	}
	if got := assemble(t, "es-AR", "identidad", []fs.FS{root}); got != "hola" {
		t.Fatalf("es-AR quedó %q", got)
	}
}

func TestAMissingTranslationFallsBackToTheDefault(t *testing.T) {
	root := dir(t, map[string]string{
		"es-AR/uno": "primero", "en/uno": "first", "es-AR/dos": "segundo", "en/tres": "third",
	})
	if got := assemble(t, "en", "uno, dos", []fs.FS{root}); got != "first\n\nsegundo" {
		t.Fatalf("quedó %q", got)
	}
	if _, err := Assemble("es-AR", "tres", []fs.FS{root}, nil); err == nil {
		t.Fatal("el idioma por defecto no debería ver lo que sólo tiene el otro")
	}
}

func TestTheLanguageGoesBeforeTheDefault(t *testing.T) {
	mine := dir(t, map[string]string{"es-AR/identidad": "mía"})
	shipped := dir(t, map[string]string{"en/identidad": "de fábrica"})
	if got := assemble(t, "en", "identidad", []fs.FS{mine, shipped}); got != "de fábrica" {
		t.Fatalf("quedó %q", got)
	}
	if got := assemble(t, "es-AR", "identidad", []fs.FS{mine, shipped}); got != "mía" {
		t.Fatalf("quedó %q", got)
	}
}

func TestTheShippedFragmentsAssembleWithTheDefaultSpec(t *testing.T) {
	got, err := Assemble("", Default, []fs.FS{Builtin}, []Var{
		{"usuario", "Ana"}, {"asistente", "Jimmy"}, {"skills", "No hay ninguna instalada."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Sos Jimmy, el asistente de Ana.") {
		t.Fatalf("no armó la identidad:\n%.200s", got)
	}
	for _, heading := range []string{"## Workspace", "## Memoria", "## Agenda", "## Skills"} {
		if !strings.Contains(got, heading) {
			t.Fatalf("falta %q en:\n%.400s", heading, got)
		}
	}
	if strings.Contains(got, "{{") {
		t.Fatalf("quedó un placeholder sin llenar")
	}
}

func TestParseVarsReadsKeyValuePairs(t *testing.T) {
	want := []Var{{"usuario", "Ana"}, {"asistente", "Jimmy"}}
	if got := ParseVars("usuario=Ana, asistente = Jimmy"); !reflect.DeepEqual(got, want) {
		t.Fatalf("quedó %+v", got)
	}
	if got := ParseVars("basura"); !reflect.DeepEqual(got, []Var{}) {
		t.Fatalf("quedó %+v", got)
	}
	if got := ParseVars("clave=a=b"); !reflect.DeepEqual(got, []Var{{"clave", "a=b"}}) {
		t.Fatalf("quedó %+v", got)
	}
}

// fixtureRoot arma los mismos fragmentos que el arnés de Rust que generó
// testdata/paridad-rust.txt; los nombres y el contenido tienen que coincidir.
func fixtureRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "root")
	fragmentIn(t, root, "order/es-AR/uno", "primero\n")
	fragmentIn(t, root, "order/es-AR/dos", "segundo\n")
	fragmentIn(t, root, "over/es-AR/identidad", "mía")
	fragmentIn(t, root, "under/es-AR/identidad", "de fábrica")
	fragmentIn(t, root, "empty/es-AR/identidad", "   \n")
	fragmentIn(t, root, "fill/es-AR/identidad", "Sos {{asistente}}, el asistente de {{usuario}}.\n")
	fragmentIn(t, root, "braces/es-AR/herramientas", "`[{\"action\":\"goto\"}]`\n")
	fragmentIn(t, root, "three/es-AR/a", "\n\n  primero  \n\n")
	fragmentIn(t, root, "three/es-AR/b", "segundo")
	fragmentIn(t, root, "three/es-AR/c", "  \n  tercero\n")
	fragmentIn(t, root, "missing/es-AR/no-var", "Hola {{usuario}}")
	fragmentIn(t, root, "missing/es-AR/abierto", "Hola {{usuario")
	return root
}

func escape(text string) string {
	return strings.NewReplacer("\\", "\\\\", "\n", "\\n", "\t", "\\t").Replace(text)
}

func readTestdata(t *testing.T, path string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]string{}
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		name, value, found := strings.Cut(line, "\t")
		if !found {
			t.Fatalf("línea rara en el dump: %q", line)
		}
		expected[name] = value
	}
	return expected
}

// TestParidadConElRust corre los mismos casos que el arnés que compila
// jimmy/src/prompt.rs suelto, con los dirs apuntados al `es-AR` de cada uno,
// y compara byte a byte. Falla también si un caso del dump no tiene
// contraparte acá, que es como el dump se queda viejo.
func TestParidadConElRust(t *testing.T) {
	root := fixtureRoot(t)
	at := func(name string) fs.FS { return os.DirFS(filepath.Join(root, name)) }
	outcome := func(out string, err error) string {
		if err != nil {
			return "error"
		}
		return escape(out)
	}
	cases := map[string]func() string{
		"join_order": func() string {
			return outcome(Assemble("es-AR", "uno, dos", []fs.FS{at("order")}, nil))
		},
		"earlier_dir_wins": func() string {
			return outcome(Assemble("es-AR", "identidad", []fs.FS{at("over"), at("under")}, nil))
		},
		"empty_spec": func() string {
			return outcome(Assemble("es-AR", "  ", nil, nil))
		},
		"empty_file_is_skipped": func() string {
			return outcome(Assemble("es-AR", "identidad", []fs.FS{at("empty"), at("under")}, nil))
		},
		"fill": func() string {
			return outcome(Assemble("es-AR", "identidad", []fs.FS{at("fill")},
				[]Var{{"usuario", "Ana"}, {"asistente", "Jimmy"}}))
		},
		"single_braces": func() string {
			return outcome(Assemble("es-AR", "herramientas", []fs.FS{at("braces")},
				[]Var{{"usuario", "Ana"}}))
		},
		"three_fragments": func() string {
			return outcome(Assemble("es-AR", "a, b, c", []fs.FS{at("three")}, nil))
		},
		"missing_fragment": func() string {
			return outcome(Assemble("es-AR", "no-existe", []fs.FS{at("order")}, nil))
		},
		"missing_var": func() string {
			return outcome(Assemble("es-AR", "no-var", []fs.FS{at("missing")}, nil))
		},
		"unclosed_placeholder": func() string {
			return outcome(Assemble("es-AR", "abierto", []fs.FS{at("missing")},
				[]Var{{"usuario", "Ana"}}))
		},
		"parse_vars": func() string {
			pairs := []string{}
			for _, variable := range ParseVars("usuario=Ana, asistente = Jimmy, basura") {
				pairs = append(pairs, fmt.Sprintf("(%q, %q)", variable.Name, variable.Value))
			}
			return "[" + strings.Join(pairs, ", ") + "]"
		},
	}
	expected := readTestdata(t, "testdata/paridad-rust.txt")
	for name, want := range expected {
		run, found := cases[name]
		if !found {
			t.Fatalf("el dump tiene el caso %q y acá no está", name)
		}
		if got := run(); got != want {
			t.Fatalf("%s: el Rust dijo %q y acá quedó %q", name, want, got)
		}
	}
	for name := range cases {
		if _, found := expected[name]; !found {
			t.Fatalf("el caso %q no está en el dump", name)
		}
	}
}

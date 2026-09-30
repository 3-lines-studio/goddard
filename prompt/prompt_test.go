package prompt

import (
	"fmt"
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

func assemble(t *testing.T, language, spec string, dirs []string, vars ...Var) string {
	t.Helper()
	out, err := Assemble(language, spec, dirs, vars)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	return out
}

func TestJoinsTheFragmentsInSpecOrder(t *testing.T) {
	dir := t.TempDir()
	fragmentIn(t, dir, "uno", "primero\n")
	fragmentIn(t, dir, "dos", "segundo\n")
	if got := assemble(t, "", "uno, dos", []string{dir}); got != "primero\n\nsegundo" {
		t.Fatalf("quedó %q", got)
	}
}

func TestAnEarlierDirWins(t *testing.T) {
	over := t.TempDir()
	under := t.TempDir()
	fragmentIn(t, over, "identidad", "mía")
	fragmentIn(t, under, "identidad", "de fábrica")
	if got := assemble(t, "", "identidad", []string{over, under}); got != "mía" {
		t.Fatalf("quedó %q", got)
	}
}

func TestAnEmptySpecAssemblesNothing(t *testing.T) {
	if got := assemble(t, "", "  ", nil); got != "" {
		t.Fatalf("quedó %q", got)
	}
}

func TestAMissingFragmentIsAnError(t *testing.T) {
	if _, err := Assemble("", "no-existe", []string{t.TempDir()}, nil); err == nil {
		t.Fatal("no se quejó")
	}
}

func TestAnEmptyFragmentFileIsIgnored(t *testing.T) {
	empty := t.TempDir()
	under := t.TempDir()
	fragmentIn(t, empty, "identidad", "   \n")
	fragmentIn(t, under, "identidad", "de fábrica")
	if got := assemble(t, "", "identidad", []string{empty, under}); got != "de fábrica" {
		t.Fatalf("quedó %q", got)
	}
}

func TestFillsTheVariables(t *testing.T) {
	dir := t.TempDir()
	fragmentIn(t, dir, "identidad", "Sos {{asistente}}, el asistente de {{usuario}}.\n")
	got := assemble(t, "", "identidad", []string{dir}, Var{"usuario", "Ana"}, Var{"asistente", "Jimmy"})
	if got != "Sos Jimmy, el asistente de Ana." {
		t.Fatalf("quedó %q", got)
	}
}

func TestLeavesSingleBracesAlone(t *testing.T) {
	dir := t.TempDir()
	fragmentIn(t, dir, "herramientas", "`[{\"action\":\"goto\"}]`\n")
	if got := assemble(t, "", "herramientas", []string{dir}); got != "`[{\"action\":\"goto\"}]`" {
		t.Fatalf("quedó %q", got)
	}
}

func TestAMissingVariableIsAnError(t *testing.T) {
	dir := t.TempDir()
	fragmentIn(t, dir, "identidad", "Hola {{usuario}}")
	_, err := Assemble("", "identidad", []string{dir}, nil)
	if err == nil || !strings.Contains(err.Error(), "usuario") {
		t.Fatalf("dijo %v", err)
	}
}

func TestAnUnclosedPlaceholderIsAnError(t *testing.T) {
	dir := t.TempDir()
	fragmentIn(t, dir, "identidad", "Hola {{usuario")
	if _, err := Assemble("", "identidad", []string{dir}, []Var{{"usuario", "Ana"}}); err == nil {
		t.Fatal("no se quejó")
	}
}

func TestTheLanguageAsksForItsOwnFragments(t *testing.T) {
	dir := t.TempDir()
	fragmentIn(t, dir, "identidad", "hola")
	fragmentIn(t, dir, "en/identidad", "hello\n")
	if got := assemble(t, "en", "identidad", []string{dir}); got != "hello" {
		t.Fatalf("en quedó %q", got)
	}
	if got := assemble(t, "es", "identidad", []string{dir}); got != "hola" {
		t.Fatalf("es quedó %q", got)
	}
	if got := assemble(t, "", "identidad", []string{dir}); got != "hola" {
		t.Fatalf("sin idioma quedó %q", got)
	}
}

func TestADirWinsOverTheLanguage(t *testing.T) {
	over := t.TempDir()
	under := t.TempDir()
	fragmentIn(t, over, "identidad", "mía")
	fragmentIn(t, under, "en/identidad", "de fábrica")
	if got := assemble(t, "en", "identidad", []string{over, under}); got != "mía" {
		t.Fatalf("quedó %q", got)
	}
}

func TestAMissingTranslationFallsBackToTheDefault(t *testing.T) {
	dir := t.TempDir()
	fragmentIn(t, dir, "uno", "primero")
	fragmentIn(t, dir, "en/uno", "first")
	fragmentIn(t, dir, "dos", "segundo")
	if got := assemble(t, "en", "uno, dos", []string{dir}); got != "first\n\nsegundo" {
		t.Fatalf("quedó %q", got)
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

func TestDirsPutsTheWorkspaceFirst(t *testing.T) {
	got := Dirs("/data", "/usr/local/share/goddard/prompts")
	want := []string{filepath.Join("/data", "prompts"), "/usr/local/share/goddard/prompts"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("quedó %+v", got)
	}
}

// fixtureRoot arma los mismos fragmentos que el arnés de Rust que generó
// testdata/paridad-rust.txt; los nombres y el contenido tienen que coincidir.
func fixtureRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "root")
	fragmentIn(t, root, "order/uno", "primero\n")
	fragmentIn(t, root, "order/dos", "segundo\n")
	fragmentIn(t, root, "over/identidad", "mía")
	fragmentIn(t, root, "under/identidad", "de fábrica")
	fragmentIn(t, root, "empty/identidad", "   \n")
	fragmentIn(t, root, "fill/identidad", "Sos {{asistente}}, el asistente de {{usuario}}.\n")
	fragmentIn(t, root, "braces/herramientas", "`[{\"action\":\"goto\"}]`\n")
	fragmentIn(t, root, "three/a", "\n\n  primero  \n\n")
	fragmentIn(t, root, "three/b", "segundo")
	fragmentIn(t, root, "three/c", "  \n  tercero\n")
	fragmentIn(t, root, "missing/no-var", "Hola {{usuario}}")
	fragmentIn(t, root, "missing/abierto", "Hola {{usuario")
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
// jimmy/src/prompt.rs suelto y compara byte a byte. Falla también si un caso
// del dump no tiene contraparte acá, que es como el dump se queda viejo.
func TestParidadConElRust(t *testing.T) {
	root := fixtureRoot(t)
	at := func(name string) string { return filepath.Join(root, name) }
	outcome := func(out string, err error) string {
		if err != nil {
			return "error"
		}
		return escape(out)
	}
	cases := map[string]func() string{
		"join_order": func() string {
			return outcome(Assemble("", "uno, dos", []string{at("order")}, nil))
		},
		"earlier_dir_wins": func() string {
			return outcome(Assemble("", "identidad", []string{at("over"), at("under")}, nil))
		},
		"empty_spec": func() string {
			return outcome(Assemble("", "  ", nil, nil))
		},
		"empty_file_is_skipped": func() string {
			return outcome(Assemble("", "identidad", []string{at("empty"), at("under")}, nil))
		},
		"fill": func() string {
			return outcome(Assemble("", "identidad", []string{at("fill")},
				[]Var{{"usuario", "Ana"}, {"asistente", "Jimmy"}}))
		},
		"single_braces": func() string {
			return outcome(Assemble("", "herramientas", []string{at("braces")},
				[]Var{{"usuario", "Ana"}}))
		},
		"three_fragments": func() string {
			return outcome(Assemble("", "a, b, c", []string{at("three")}, nil))
		},
		"missing_fragment": func() string {
			return outcome(Assemble("", "no-existe", []string{at("order")}, nil))
		},
		"missing_var": func() string {
			return outcome(Assemble("", "no-var", []string{at("missing")}, nil))
		},
		"unclosed_placeholder": func() string {
			return outcome(Assemble("", "abierto", []string{at("missing")},
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

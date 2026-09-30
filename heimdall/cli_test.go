package heimdall

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func args(line string) []string {
	return strings.Fields(line)
}

func TestFlagsReadTheValueAfterTheName(t *testing.T) {
	line := args("run --project bifrost --env dev -- echo hola")
	if project, ok := flag(line, "--project"); !ok || project != "bifrost" {
		t.Fatalf("--project dio %q %v", project, ok)
	}
	if env, ok := flag(line, "--env"); !ok || env != "dev" {
		t.Fatalf("--env dio %q %v", env, ok)
	}
	if _, ok := flag(line, "--nada"); ok {
		t.Fatal("encontró un flag que no está")
	}
}

func TestFlagsAlsoReadTheValueAfterAnEqualsSign(t *testing.T) {
	line := args("run --config=dev -- echo hola")
	if env, ok := flag(line, "--config"); !ok || env != "dev" {
		t.Fatalf("--config dio %q %v", env, ok)
	}
}

func TestTheDopplerShortFlagsWork(t *testing.T) {
	line := args("run -p picsel -c dev -- echo hola")
	if project := pick(line, []string{"--project", "-p"}, ""); project != "picsel" {
		t.Fatalf("-p dio %q", project)
	}
	if env := pick(line, []string{"--config", "-c"}, ""); env != "dev" {
		t.Fatalf("-c dio %q", env)
	}
}

func TestAFlagBeatsTheSetupFile(t *testing.T) {
	line := args("run -c prd -- echo hola")
	if env := pick(line, []string{"--config", "-c"}, "dev"); env != "prd" {
		t.Fatalf("dio %q", env)
	}
}

func TestTheSetupFileFillsWhatTheFlagsLeaveOut(t *testing.T) {
	line := args("run -- echo hola")
	if env := pick(line, []string{"--config", "-c"}, "dev"); env != "dev" {
		t.Fatalf("dio %q", env)
	}
}

func TestTheSetupFileGivesTheProjectAndTheEnvironment(t *testing.T) {
	text := "setup:\n  - project: picsel\n    config: dev\n"
	project, ok := yamlValue(text, "project")
	if !ok || project != "picsel" {
		t.Fatalf("project dio %q %v", project, ok)
	}
	env, ok := yamlValue(text, "config")
	if !ok || env != "dev" {
		t.Fatalf("config dio %q %v", env, ok)
	}
	if _, ok := yamlValue(text, "token"); ok {
		t.Fatal("encontró una clave que no está")
	}
}

func TestTheCommandGoesAfterTheSeparator(t *testing.T) {
	line := args("run --project bifrost --env dev -- echo hola mundo")
	separator := separatorIndex(line)
	if separator < 0 {
		t.Fatal("no encontró el separador")
	}
	if got := strings.Join(line[separator+1:], " "); got != "echo hola mundo" {
		t.Fatalf("el comando quedó %q", got)
	}
}

func TestARepeatedFlagDoesNotMoveTheSeparator(t *testing.T) {
	line := args("run --preserve-env --preserve-env -- echo hola")
	separator := separatorIndex(line)
	if separator < 0 {
		t.Fatal("no encontró el separador")
	}
	if got := strings.Join(line[separator+1:], " "); got != "echo hola" {
		t.Fatalf("el comando quedó %q", got)
	}
}

func TestAMissingFlagIsAnError(t *testing.T) {
	if _, ok := flag(args("ls"), "--project"); ok {
		t.Fatal("encontró un flag en una línea sin flags")
	}
}

func TestTTLUnitsBecomeSeconds(t *testing.T) {
	casos := []struct {
		text string
		want int64
	}{
		{"30s", 30},
		{"15m", 900},
		{"2h", 7200},
		{"7d", 604800},
	}
	for _, caso := range casos {
		got, err := parseTTL(caso.text)
		if err != nil {
			t.Fatalf("%s: %v", caso.text, err)
		}
		if got != caso.want {
			t.Errorf("%s dio %d want %d", caso.text, got, caso.want)
		}
	}
}

func TestABadTTLIsAnError(t *testing.T) {
	for _, text := range []string{"2", "h", "2w", "dos-h"} {
		if _, err := parseTTL(text); err == nil {
			t.Errorf("aceptó %q", text)
		}
	}
}

func TestTheChildKeepsWhatTheShellHad(t *testing.T) {
	parent := []string{"A=la-del-shell", "HEIMDALL_TOKEN=" + "hd_" + "secreto", "B=1"}
	secrets := map[string]any{"A": "la-del-store", "C": "3", "RARO": 7}
	got := mergeEnv(parent, secrets)
	if !reflect.DeepEqual(got, []string{"A=la-del-shell", "B=1", "C=3"}) {
		t.Fatalf("el entorno del hijo quedó %v", got)
	}
}

func TestTheSetupFileIsFoundUpwards(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, setupFile), []byte("setup:\n  - project: picsel\n    config: dev\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Chdir(sub)
	file := findSetup()
	if file.project != "picsel" || file.env != "dev" {
		t.Fatalf("encontró %v", file)
	}
	project, env, err := scope([]string{"run", "--", "echo", "hola"})
	if err != nil {
		t.Fatalf("scope: %v", err)
	}
	if project != "picsel" || env != "dev" {
		t.Fatalf("el alcance quedó %s/%s", project, env)
	}
}

func TestSetupWritesTheFile(t *testing.T) {
	dir := t.TempDir()
	if err := setup([]string{"setup", "--project", "picsel", "--config", "dev", "--dir", dir}); err != nil {
		t.Fatalf("setup: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, setupFile))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if string(raw) != "setup:\n  - project: picsel\n    config: dev\n" {
		t.Fatalf("escribió %q", raw)
	}
}

func TestLoginLeavesTheTokenClosedAndLogoutTakesItAway(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := login([]string{"login", "--token", "hd_" + "guardado"}); err != nil {
		t.Fatalf("login: %v", err)
	}
	file := filepath.Join(home, ".heimdall", "token")
	info, err := os.Stat(file)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("el token quedó en %v", info.Mode().Perm())
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if string(raw) != "hd_"+"guardado" {
		t.Fatalf("guardó %q", raw)
	}
	if err := logout(); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("el token quedó ahí")
	}
}

func TestForwardBringsTheSecretsFromTheAPI(t *testing.T) {
	asked := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.RequestURI() + " " + r.Header.Get("Authorization")
		w.Write([]byte(`{"BUCKET":"picsel-staging","A":"la-del-store"}`))
	}))
	defer server.Close()
	t.Setenv("HEIMDALL_URL", server.URL)
	t.Setenv("HEIMDALL_TOKEN", "hd_"+"de-prueba")
	child, err := childEnv("picsel", "dev")
	if err != nil {
		t.Fatalf("childEnv: %v", err)
	}
	if asked != "/v1/secrets?project=picsel&env=dev Bearer hd_de-prueba" {
		t.Fatalf("pidió %q", asked)
	}
	environment := map[string]bool{}
	for _, entry := range child {
		environment[entry] = true
	}
	if !environment["A=la-del-store"] || !environment["BUCKET=picsel-staging"] {
		t.Fatalf("el entorno quedó con %v", environment)
	}
	if environment["HEIMDALL_TOKEN=hd_de-prueba"] {
		t.Fatal("el token del store viajó al hijo")
	}
}

func TestTheAPIErrorIsWhatTheCallerSees(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte(`{"error":"este token no llega a ese entorno"}`))
	}))
	defer server.Close()
	t.Setenv("HEIMDALL_URL", server.URL)
	t.Setenv("HEIMDALL_TOKEN", "hd_"+"de-prueba")
	_, err := childEnv("bifrost", "prod")
	if err == nil {
		t.Fatal("no se quejó")
	}
	if err.Error() != "este token no llega a ese entorno" {
		t.Fatalf("dijo %q", err)
	}
}

package heimdall

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const setupFile = "heimdall.yaml"

// Run is the command line: it speaks the same language as `doppler run`, so
// swapping one for the other is a find and replace.
func Run(args []string) error {
	if len(args) == 0 {
		usage()
		return nil
	}
	switch args[0] {
	case "run":
		return forward(args)
	case "setup":
		return setup(args)
	case "login":
		return login(args)
	case "logout":
		return logout()
	case "set":
		return setSecret(args)
	case "unset":
		return unsetSecret(args)
	case "ls", "keys":
		return listKeys(args)
	case "environments", "envs":
		return listEnvironments()
	case "token":
		return token(args)
	case "audit":
		return listAudit()
	case "help":
		usage()
		return nil
	}
	return fmt.Errorf("no conozco «%s», mirá `heimdall help`", args[0])
}

func usage() {
	fmt.Print(`heimdall — secretos por proyecto y entorno

  heimdall setup --project X --config Y [--dir D]
  heimdall run [--project X] [--config Y] -- comando
  heimdall login --token T
  heimdall logout
  heimdall set CLAVE=valor [--project X] [--env Y]
  heimdall unset CLAVE [--project X] [--env Y]
  heimdall ls [--project X] [--env Y]
  heimdall environments
  heimdall token create --name N [--project X] [--env Y] [--keys A,B] [--ttl 2h]
                        (el proyecto y el entorno aceptan «*»)
  heimdall token create --name N --admin [--ttl 24h]
  heimdall token list
  heimdall token revoke --id ID
  heimdall audit
  heimdall help

El proyecto y el entorno salen de los flags o, si no los pasás, de un
heimdall.yaml que se busca en el directorio actual y hacia arriba. Lo escribe
«heimdall setup». Los flags son los de doppler: -p/--project, -c/--config.

El cliente lee HEIMDALL_URL (por defecto http://127.0.0.1:8080) y HEIMDALL_TOKEN.
El token también puede estar guardado con «heimdall login».
`)
}

// forward puts whatever the token sees into the command's environment and takes
// HEIMDALL_TOKEN out of it, so the child cannot read the store on its own.
// Whatever the shell already defines wins.
func forward(args []string) error {
	separator := separatorIndex(args)
	if separator < 0 {
		return errors.New("falta el -- antes del comando")
	}
	project, env, err := scope(args[:separator])
	if err != nil {
		return err
	}
	command := args[separator+1:]
	if len(command) == 0 {
		return errors.New("falta el comando después del --")
	}
	environment, err := childEnv(project, env)
	if err != nil {
		return err
	}
	return execCommand(command, environment)
}

func separatorIndex(args []string) int {
	for index, arg := range args {
		if arg == "--" {
			return index
		}
	}
	return -1
}

func childEnv(project, env string) ([]string, error) {
	value, err := call("GET", fmt.Sprintf("/v1/secrets?project=%s&env=%s", project, env), nil)
	if err != nil {
		return nil, err
	}
	secrets, _ := value.(map[string]any)
	return mergeEnv(os.Environ(), secrets), nil
}

func mergeEnv(parent []string, secrets map[string]any) []string {
	environment := []string{}
	present := map[string]bool{}
	for _, entry := range parent {
		name, _, _ := strings.Cut(entry, "=")
		present[name] = true
		if name == "HEIMDALL_TOKEN" {
			continue
		}
		environment = append(environment, entry)
	}
	names := make([]string, 0, len(secrets))
	for name := range secrets {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		text, ok := secrets[name].(string)
		if !ok || present[name] {
			continue
		}
		environment = append(environment, name+"="+text)
	}
	return environment
}

func setup(args []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("no sé dónde estoy: %v", err)
	}
	if value, ok := flag(args, "--dir"); ok {
		dir = value
	}
	project := pick(args, []string{"--project", "-p"}, "")
	if project == "" {
		return errors.New("falta --project")
	}
	env := pick(args, []string{"--config", "-c", "--env"}, "")
	if env == "" {
		return errors.New("falta --config")
	}
	file := filepath.Join(dir, setupFile)
	body := fmt.Sprintf("setup:\n  - project: %s\n    config: %s\n", project, env)
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		return fmt.Errorf("no pude escribir %s: %v", file, err)
	}
	fmt.Printf("%s → %s/%s\n", file, project, env)
	return nil
}

func login(args []string) error {
	token, ok := flag(args, "--token")
	if !ok {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("no pude leer el token: %v", err)
		}
		token = strings.TrimSpace(line)
	}
	if token == "" {
		return errors.New("esperaba el token: `heimdall login --token T`, o pegámelo por stdin")
	}
	file := tokenFile()
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return fmt.Errorf("no pude crear %s: %v", filepath.Dir(file), err)
	}
	if err := os.WriteFile(file, []byte(token), 0o600); err != nil {
		return fmt.Errorf("no pude escribir %s: %v", file, err)
	}
	if err := os.Chmod(file, 0o600); err != nil {
		return fmt.Errorf("no pude cerrar los permisos de %s: %v", file, err)
	}
	fmt.Printf("guardado en %s\n", file)
	return nil
}

func logout() error {
	file := tokenFile()
	if _, err := os.Stat(file); err == nil {
		if err := os.Remove(file); err != nil {
			return fmt.Errorf("no pude borrar %s: %v", file, err)
		}
	}
	fmt.Println("listo, no queda ningún token guardado")
	return nil
}

func scope(args []string) (string, string, error) {
	file := findSetup()
	project := pick(args, []string{"--project", "-p"}, file.project)
	if project == "" {
		return "", "", errors.New("falta el proyecto: pasá --project, o corré «heimdall setup»")
	}
	env := pick(args, []string{"--config", "-c", "--env"}, file.env)
	if env == "" {
		return "", "", errors.New("falta el entorno: pasá --config, o corré «heimdall setup»")
	}
	return project, env, nil
}

func pick(args []string, names []string, fallback string) string {
	for _, name := range names {
		if value, ok := flag(args, name); ok {
			return value
		}
	}
	return fallback
}

func flag(args []string, name string) (string, bool) {
	for index, arg := range args {
		if arg == name {
			if index+1 >= len(args) {
				return "", false
			}
			return args[index+1], true
		}
		if value, ok := strings.CutPrefix(arg, name+"="); ok {
			return value, true
		}
	}
	return "", false
}

// setupYAML is the project and the environment a repository declares.
type setupYAML struct {
	project string
	env     string
}

// findSetup walks up from the working directory: a subdirectory works the same
// as the root.
func findSetup() setupYAML {
	dir, err := os.Getwd()
	if err != nil {
		return setupYAML{}
	}
	for {
		file := filepath.Join(dir, setupFile)
		if info, err := os.Stat(file); err == nil && info.Mode().IsRegular() {
			return readSetup(file)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return setupYAML{}
		}
		dir = parent
	}
}

func readSetup(file string) setupYAML {
	text, err := os.ReadFile(file)
	if err != nil {
		return setupYAML{}
	}
	project, ok := yamlValue(string(text), "project")
	if !ok {
		return setupYAML{}
	}
	env, ok := yamlValue(string(text), "config")
	if !ok {
		return setupYAML{}
	}
	return setupYAML{project: project, env: env}
}

func yamlValue(text, key string) (string, bool) {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "-"))
		if value, ok := strings.CutPrefix(line, key+":"); ok {
			return strings.TrimSpace(value), true
		}
	}
	return "", false
}

func setSecret(args []string) error {
	project, env, err := scope(args)
	if err != nil {
		return err
	}
	if len(args) < 2 || !strings.Contains(args[1], "=") {
		return errors.New("esperaba CLAVE=valor")
	}
	name, value, _ := strings.Cut(args[1], "=")
	body := map[string]any{"project": project, "env": env, "key": name, "value": value}
	if _, err := call("PUT", "/v1/secrets", body); err != nil {
		return err
	}
	fmt.Printf("%s guardada en %s/%s\n", name, project, env)
	return nil
}

func unsetSecret(args []string) error {
	project, env, err := scope(args)
	if err != nil {
		return err
	}
	if len(args) < 2 {
		return errors.New("esperaba la clave a sacar")
	}
	name := args[1]
	body := map[string]any{"project": project, "env": env, "key": name}
	if _, err := call("DELETE", "/v1/secrets", body); err != nil {
		return err
	}
	fmt.Printf("%s ya no está en %s/%s\n", name, project, env)
	return nil
}

func listKeys(args []string) error {
	project, env, err := scope(args)
	if err != nil {
		return err
	}
	value, err := call("GET", fmt.Sprintf("/v1/keys?project=%s&env=%s", project, env), nil)
	if err != nil {
		return err
	}
	for _, entry := range array(value) {
		fmt.Println(text(entry))
	}
	return nil
}

func listEnvironments() error {
	value, err := call("GET", "/v1/environments", nil)
	if err != nil {
		return err
	}
	for _, entry := range array(value) {
		fmt.Println(text(entry))
	}
	return nil
}

func token(args []string) error {
	subcommand := "list"
	if len(args) > 1 {
		subcommand = args[1]
	}
	switch subcommand {
	case "create":
		return tokenCreate(args)
	case "revoke":
		return tokenRevoke(args)
	case "list":
		return tokenList()
	}
	return fmt.Errorf("no conozco `token %s`", subcommand)
}

func tokenCreate(args []string) error {
	name, ok := flag(args, "--name")
	if !ok {
		return errors.New("falta --name")
	}
	admin := false
	for _, arg := range args {
		if arg == "--admin" {
			admin = true
		}
	}
	body := map[string]any{"name": name, "admin": admin}
	if !admin {
		project, env, err := scope(args)
		if err != nil {
			return err
		}
		body["project"] = project
		body["env"] = env
		if keys, ok := flag(args, "--keys"); ok {
			list := []string{}
			for _, key := range strings.Split(keys, ",") {
				if key = strings.TrimSpace(key); key != "" {
					list = append(list, key)
				}
			}
			body["keys"] = list
		}
	}
	if ttl, ok := flag(args, "--ttl"); ok {
		seconds, err := parseTTL(ttl)
		if err != nil {
			return err
		}
		body["ttl"] = seconds
	}
	value, err := call("POST", "/v1/tokens", body)
	if err != nil {
		return err
	}
	created := object(value)
	fmt.Println(text(created["token"]))
	fmt.Fprintf(os.Stderr, "guardalo: no se vuelve a mostrar. id %s.\n", text(created["id"]))
	return nil
}

func parseTTL(plain string) (int64, error) {
	if len(plain) < 2 {
		return 0, fmt.Errorf("«%s»: usá un número y s, m, h o d", plain)
	}
	number, unit := plain[:len(plain)-1], plain[len(plain)-1:]
	value, err := strconv.ParseInt(number, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("«%s» no tiene un número adelante", plain)
	}
	switch unit {
	case "s":
		return value, nil
	case "m":
		return value * 60, nil
	case "h":
		return value * 60 * 60, nil
	case "d":
		return value * 24 * 60 * 60, nil
	}
	return 0, fmt.Errorf("«%s»: usá s, m, h o d", plain)
}

func tokenRevoke(args []string) error {
	id, ok := flag(args, "--id")
	if !ok {
		return errors.New("falta --id")
	}
	value, err := call("DELETE", "/v1/tokens?id="+id, nil)
	if err != nil {
		return err
	}
	name := text(object(value)["name"])
	if name == "" {
		name = id
	}
	fmt.Printf("revocado %s\n", name)
	return nil
}

func tokenList() error {
	value, err := call("GET", "/v1/tokens", nil)
	if err != nil {
		return err
	}
	for _, entry := range array(value) {
		fmt.Printf("%s  %s\n", text(object(entry)["id"]), describe(object(entry)))
	}
	return nil
}

func describe(entry map[string]any) string {
	name := text(entry["name"])
	scope := ""
	if entry["admin"] == true {
		scope = "admin"
	} else {
		scope = text(entry["project"]) + "/" + text(entry["env"])
	}
	keys := []string{}
	for _, key := range array(entry["keys"]) {
		keys = append(keys, text(key))
	}
	what := "ve todo el entorno"
	switch {
	case entry["keys"] == nil:
	case len(keys) == 0:
		what = "sin claves: ve todo el entorno"
	default:
		what = "claves: " + strings.Join(keys, ",")
	}
	out := fmt.Sprintf("%s  %s  %s", name, scope, what)
	if expires, ok := number(entry["expires_at"]); ok {
		out += fmt.Sprintf("  vence %d", expires)
	}
	if used, ok := number(entry["last_used"]); ok {
		out += fmt.Sprintf("  último uso %d", used)
	} else {
		out += "  sin uso"
	}
	return out
}

func listAudit() error {
	value, err := call("GET", "/v1/audit", nil)
	if err != nil {
		return err
	}
	for _, entry := range array(value) {
		row := object(entry)
		at, _ := number(row["at"])
		fmt.Printf("%d  %s  %s  %s/%s  %s\n",
			at, text(row["actor"]), text(row["action"]), text(row["project"]), text(row["env"]), text(row["key"]))
	}
	return nil
}

func call(method, path string, body any) (any, error) {
	url := baseURL() + path
	token, err := authToken()
	if err != nil {
		return nil, err
	}
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("respuesta rota: %v", err)
		}
		payload = strings.NewReader(string(encoded))
	}
	request, err := http.NewRequest(method, url, payload)
	if err != nil {
		return nil, fmt.Errorf("no pude hablar con %s: %v", url, err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("no pude hablar con %s: %v", url, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var failure struct {
			Error string `json:"error"`
		}
		json.NewDecoder(response.Body).Decode(&failure)
		if failure.Error == "" {
			return nil, errors.New("la API contestó mal")
		}
		return nil, errors.New(failure.Error)
	}
	var value any
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		return nil, fmt.Errorf("respuesta rota: %v", err)
	}
	return value, nil
}

var client = &http.Client{Timeout: 30 * time.Second}

func baseURL() string {
	if url := os.Getenv("HEIMDALL_URL"); url != "" {
		return url
	}
	return "http://127.0.0.1:8080"
}

func authToken() (string, error) {
	if token := os.Getenv("HEIMDALL_TOKEN"); token != "" {
		return token, nil
	}
	file := tokenFile()
	raw, err := os.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("falta HEIMDALL_TOKEN, o guardalo con «heimdall login» en %s", file)
	}
	return strings.TrimSpace(string(raw)), nil
}

func tokenFile() string {
	home := os.Getenv("HOME")
	if home == "" {
		home = "."
	}
	return filepath.Join(home, ".heimdall", "token")
}

func object(value any) map[string]any {
	object, _ := value.(map[string]any)
	return object
}

func array(value any) []any {
	array, _ := value.([]any)
	return array
}

func text(value any) string {
	text, _ := value.(string)
	return text
}

func number(value any) (int64, bool) {
	number, ok := value.(float64)
	if !ok {
		return 0, false
	}
	return int64(number), true
}

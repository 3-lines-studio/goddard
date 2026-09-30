// Package prompt assembles a system prompt out of fragments: a spec names the
// pieces in order, each piece is a markdown file with {{variables}}, and the
// language picks which file answers. Port of jimmy's `src/prompt.rs`, with the
// language in it.
//
// A fragment is looked up as `<dir>/<language>/<name>.md` and then as
// `<dir>/<name>.md`: the file without a language is the default one, and it is
// what answers for a translation that is not there. Dirs go in order, so the
// first one with the fragment wins.
package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Var is one value a fragment can ask for with {{nombre}}.
type Var struct {
	Name  string
	Value string
}

// ParseVars reads the `nombre=valor,nombre=valor` list an embedder hands over.
// What is not a pair is left out.
func ParseVars(spec string) []Var {
	vars := []Var{}
	for _, pair := range strings.Split(spec, ",") {
		name, value, found := strings.Cut(pair, "=")
		if !found {
			continue
		}
		vars = append(vars, Var{Name: strings.TrimSpace(name), Value: strings.TrimSpace(value)})
	}
	return vars
}

// Dirs is where the fragments live: the root's go first, so a workspace copy of
// a fragment wins over the one that ships with the binary.
func Dirs(root, builtin string) []string {
	return []string{filepath.Join(root, "prompts"), builtin}
}

// Assemble joins the fragments the spec names, in that order, two newlines
// apart, with the variables filled in. An empty language asks for the default
// fragments only. A fragment that is not there, a variable a fragment asks for
// and nobody gave, and a placeholder left open are all errors: the prompt is
// the one place where guessing is worse than failing.
func Assemble(language, spec string, dirs []string, vars []Var) (string, error) {
	parts := []string{}
	for _, name := range fragmentNames(spec) {
		text, err := fragment(language, name, dirs)
		if err != nil {
			return "", err
		}
		filled, err := fill(text, vars)
		if err != nil {
			return "", err
		}
		parts = append(parts, filled)
	}
	return strings.Join(parts, "\n\n"), nil
}

func fragmentNames(spec string) []string {
	names := []string{}
	for _, name := range strings.Split(spec, ",") {
		if name = strings.TrimSpace(name); name != "" {
			names = append(names, name)
		}
	}
	return names
}

func fragment(language, name string, dirs []string) (string, error) {
	for _, dir := range dirs {
		paths := []string{filepath.Join(dir, name+".md")}
		if language != "" {
			paths = append([]string{filepath.Join(dir, language, name+".md")}, paths...)
		}
		for _, path := range paths {
			if text, ok := readFragment(path); ok {
				return text, nil
			}
		}
	}
	return "", fmt.Errorf("no encontré el fragmento `%s.md`", name)
}

func readFragment(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	text := strings.TrimSpace(string(data))
	return text, text != ""
}

func fill(text string, vars []Var) (string, error) {
	var out strings.Builder
	rest := text
	for {
		start := strings.Index(rest, "{{")
		if start < 0 {
			out.WriteString(rest)
			return out.String(), nil
		}
		out.WriteString(rest[:start])
		tail := rest[start+2:]
		end := strings.Index(tail, "}}")
		if end < 0 {
			return "", fmt.Errorf("placeholder sin cerrar en `%s`", rest[start:])
		}
		name := strings.TrimSpace(tail[:end])
		value, found := lookup(vars, name)
		if !found {
			return "", fmt.Errorf("falta la variable `%s`", name)
		}
		out.WriteString(value)
		rest = tail[end+2:]
	}
}

func lookup(vars []Var, name string) (string, bool) {
	for _, variable := range vars {
		if variable.Name == name {
			return variable.Value, true
		}
	}
	return "", false
}

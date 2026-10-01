// Package prompt assembles a system prompt out of fragments: a spec names the
// pieces in order, each piece is a markdown file with {{variables}}, and the
// language picks which file answers. Port of jimmy's `src/prompt.rs`, with the
// languages in it.
//
// Inside every fs.FS a fragment is `<language>/<name>.md`, and the fs.FS values
// go in order: the first one that has it wins, so a directory the service puts
// first overrides the one that ships with the binary. A fragment a language
// does not have falls back to DefaultLanguage's, and a fragment nobody has is
// an error.
package prompt

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
)

// DefaultLanguage is what an empty language asks for, and the one that answers
// for a fragment a translation does not have.
const DefaultLanguage = "es-AR"

// Default is the spec: the fragments of this assistant, in order.
const Default = "identidad,estilo,codigo,jimmy,herramientas,skills,dev,workspace,memoria,agenda,git"

//go:embed prompts
var embedded embed.FS

// Builtin is the fragments that ship with the binary.
var Builtin fs.FS = mustSub(embedded, "prompts")

func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(fmt.Sprintf("prompt: %v", err))
	}
	return sub
}

// Var is one value a fragment can ask for with {{nombre}}.
type Var struct {
	Name  string
	Value string
}

// ParseVars reads the `nombre=valor,nombre=valor` list it is given.
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

// Assemble joins the fragments the spec names, in that order, two newlines
// apart, with the variables filled in. An empty language asks for the default
// one. A fragment that is not there, a variable a fragment asks for and nobody
// gave, and a placeholder left open are all errors: the prompt is the one place
// where guessing is worse than failing.
func Assemble(language, spec string, dirs []fs.FS, vars []Var) (string, error) {
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

func fragment(language, name string, dirs []fs.FS) (string, error) {
	language = languageOrDefault(language)
	for _, candidate := range languages(language) {
		for _, dir := range dirs {
			if text, ok := readFragment(dir, candidate, name); ok {
				return text, nil
			}
		}
	}
	return "", fmt.Errorf("no encontré el fragmento `%s/%s.md`", language, name)
}

func languageOrDefault(language string) string {
	if language == "" {
		return DefaultLanguage
	}
	return language
}

func languages(language string) []string {
	if language == DefaultLanguage {
		return []string{language}
	}
	return []string{language, DefaultLanguage}
}

func readFragment(dir fs.FS, language, name string) (string, bool) {
	data, err := fs.ReadFile(dir, language+"/"+name+".md")
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

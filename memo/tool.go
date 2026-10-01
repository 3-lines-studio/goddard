package memo

import (
	"context"
	"errors"
	"fmt"

	"github.com/3-lines-studio/goddard/axe"
)

const toolSchema = `{
  "type": "object",
  "properties": {
    "action": {
      "type": "string",
      "enum": ["add", "show", "list"],
      "description": "add to write a fact, show to read one, list to see the keys that exist"
    },
    "key": {"type": "string", "description": "the fact to write or read: tema, or familia/tema for a project's"},
    "kind": {"type": "string", "description": "the kind of a fact being written: decision, estado, medicion, bugfix, herramienta, identidad, proyecto or plataforma"},
    "text": {"type": "string", "description": "the fact itself, for add"}
  },
  "required": ["action"]
}`

// Tool is the memory for the agent harness: `add` writes a fact, `show` reads
// one and `list` names the keys that exist, which is what jimmy had in its
// command line. The store goes in when the service builds it, so the tool
// travels inside the binary and needs nothing on the PATH.
func Tool(store *PgStore) axe.Tool {
	type args struct {
		Action string `json:"action"`
		Key    string `json:"key"`
		Kind   string `json:"kind"`
		Text   string `json:"text"`
	}
	return axe.NewTool("memo",
		"Write down or read back a durable fact: who the user is, a decision that stands, how a repo works.",
		toolSchema,
		func(in args) string {
			text, err := run(context.Background(), store, in.Action, in.Key, in.Kind, in.Text)
			if err != nil {
				return "error: " + err.Error()
			}
			return text
		})
}

func run(ctx context.Context, store *PgStore, action, key, kind, text string) (string, error) {
	switch action {
	case "add":
		outcome, err := store.Add(ctx, key, kind, text)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s · %s · %s (%s)", key, kind, date(today()), outcome), nil
	case "show":
		if key == "" {
			return "", errors.New("decime qué clave mostrar")
		}
		found, ok, err := store.Show(ctx, key)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", fmt.Errorf("%s no está en la memoria", key)
		}
		return found, nil
	case "list":
		return store.List(ctx)
	}
	return "", fmt.Errorf("no conozco la acción %q, probá con add, show o list", action)
}

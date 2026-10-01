package skill

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
      "enum": ["list", "load"],
      "description": "list to see the skills installed, load to read one"
    },
    "name": {"type": "string", "description": "the skill to load"}
  },
  "required": ["action"]
}`

// Tool is the skill for the agent harness: `list` renders what this viewer can
// see, `load` reads one, which is what jimmy had in its command line. The store
// and the viewer go in when the app builds it, so the tool travels inside
// the binary and needs nothing on the PATH.
func Tool(store *PgStore, viewer Viewer) axe.Tool {
	type args struct {
		Action string `json:"action"`
		Name   string `json:"name"`
	}
	return axe.NewTool("skill",
		"List the skills installed, or load one into context by name.",
		toolSchema,
		func(in args) string {
			text, err := run(context.Background(), store, viewer, in.Action, in.Name)
			if err != nil {
				return "error: " + err.Error()
			}
			return text
		})
}

func run(ctx context.Context, store *PgStore, viewer Viewer, action, name string) (string, error) {
	switch action {
	case "list":
		return store.Index(ctx, viewer)
	case "load":
		if name == "" {
			return "", errors.New("decime qué skill cargar")
		}
		return store.Load(ctx, viewer, name)
	}
	return "", fmt.Errorf("no conozco la acción %q, probá con list o load", action)
}

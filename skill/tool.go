package skill

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/3-lines-studio/goddard/axe"
)

const toolSchema = `{
  "type": "object",
  "properties": {
    "action": {
      "type": "string",
      "enum": ["list", "load", "save", "remove"],
      "description": "list to see the skills installed, load to read one, save to write one, remove to delete one"
    },
    "name": {"type": "string", "description": "the skill to load, save or remove"},
    "description": {"type": "string", "description": "for save: one line saying what the skill is for"},
    "body": {"type": "string", "description": "for save: the whole skill, in markdown"}
  },
  "required": ["action"]
}`

type args struct {
	Action      string `json:"action"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Body        string `json:"body"`
}

// Tool is the skill for the agent harness: `list` renders what this viewer can
// see, `load` reads one, and `save` and `remove` write and drop the ones of
// its own user — the system's belong to nobody. The store and the viewer go in
// when the app builds it, so the tool travels inside the binary and needs
// nothing on the PATH.
func Tool(store *PgStore, viewer Viewer) axe.Tool {
	return axe.NewTool("skill",
		"List the skills installed, load one into context by name, save one of your own, or remove it.",
		toolSchema,
		func(in args) string {
			text, err := run(context.Background(), store, viewer, in)
			if err != nil {
				return "error: " + err.Error()
			}
			return text
		})
}

func run(ctx context.Context, store *PgStore, viewer Viewer, in args) (string, error) {
	switch in.Action {
	case "list":
		return store.Index(ctx, viewer)
	case "load":
		if in.Name == "" {
			return "", errors.New("decime qué skill cargar")
		}
		return store.Load(ctx, viewer, in.Name)
	case "save":
		return save(ctx, store, viewer, in)
	case "remove":
		if in.Name == "" {
			return "", errors.New("decime qué skill borrar")
		}
		gone, err := store.Delete(ctx, viewer, Owner{Kind: User, ID: viewer.User}, in.Name)
		if err != nil {
			return "", err
		}
		if !gone {
			return "", fmt.Errorf("no hay una skill tuya que se llame %q", in.Name)
		}
		return fmt.Sprintf("borrada la skill %q", in.Name), nil
	}
	return "", fmt.Errorf("no conozco la acción %q, probá con list, load, save o remove", in.Action)
}

func save(ctx context.Context, store *PgStore, viewer Viewer, in args) (string, error) {
	if in.Name == "" {
		return "", errors.New("decime el nombre de la skill")
	}
	if strings.TrimSpace(in.Description) == "" {
		return "", errors.New("decime en una línea para qué es")
	}
	if strings.TrimSpace(in.Body) == "" {
		return "", errors.New("la skill viene sin cuerpo")
	}
	skill := Skill{
		Meta: Meta{
			Owner:       Owner{Kind: User, ID: viewer.User},
			Name:        in.Name,
			Description: strings.TrimSpace(in.Description),
		},
		Body: in.Body,
	}
	if err := store.Put(ctx, viewer, skill); err != nil {
		return "", err
	}
	return fmt.Sprintf("guardada la skill %q (%d bytes)", in.Name, len(in.Body)), nil
}

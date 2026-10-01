package schedule

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
      "enum": ["add", "list", "show", "pause", "resume", "remove"],
      "description": "add to write a task, list to see the ones that exist, show to read one with its runs, pause to stop it without losing it, resume to put it back, remove to delete it"
    },
    "name": {"type": "string", "description": "the task: lowercase letters, numbers, dash and dot"},
    "when": {"type": "string", "description": "one time only, YYYY-MM-DDTHH:MM"},
    "at": {"type": "string", "description": "every day at HH:MM"},
    "every": {"type": "string", "description": "how often: 30m, 6h, 2d; units s, m, h and d"},
    "target": {"type": "string", "description": "a chat to send a copy to, if there is one"},
    "prompt": {"type": "string", "description": "what the task has to do; it does not see the conversation"},
    "silent": {"type": "boolean", "description": "only talk when there is something to say"},
    "paused": {"type": "boolean", "description": "for add: written but not running"}
  },
  "required": ["action"]
}`

type toolArgs struct {
	Action string `json:"action"`
	Name   string `json:"name"`
	When   string `json:"when"`
	At     string `json:"at"`
	Every  string `json:"every"`
	Target string `json:"target"`
	Prompt string `json:"prompt"`
	Silent bool   `json:"silent"`
	Paused bool   `json:"paused"`
}

// Tool is the agenda for the agent harness: `add` writes a task, `list` shows
// the ones of this owner, `show` reads one with its runs, `pause` stops it
// without losing it, `resume` puts it back and `remove` deletes it. Writing
// again under the same name is how a task is edited, the way rewriting its
// file was in jimmy. The store and the owner go in when the app builds
// it, so the tool travels inside the binary and needs nothing on the PATH.
func Tool(store *PgStore, userID, project string) axe.Tool {
	return axe.NewTool("schedule",
		"Schedule a task to run on its own, in a clean context, and read back what it answered.",
		toolSchema,
		func(in toolArgs) string {
			text, err := run(context.Background(), store, userID, project, in)
			if err != nil {
				return "error: " + err.Error()
			}
			return text
		})
}

func run(ctx context.Context, store *PgStore, userID, project string, in toolArgs) (string, error) {
	switch in.Action {
	case "add":
		task := Task{
			UserID:  userID,
			Project: project,
			Name:    in.Name,
			When:    in.When,
			At:      in.At,
			Every:   in.Every,
			Target:  in.Target,
			Prompt:  in.Prompt,
			Silent:  in.Silent,
			Paused:  in.Paused,
		}
		if err := store.Add(ctx, task); err != nil {
			return "", err
		}
		return "guardada: " + head(task), nil
	case "list":
		entries, err := store.List(ctx, userID, project)
		if err != nil {
			return "", err
		}
		return Render(entries), nil
	case "show":
		if in.Name == "" {
			return "", errors.New("decime qué tarea mostrar")
		}
		entry, err := store.Get(ctx, userID, project, in.Name)
		if err != nil {
			return "", err
		}
		return Show(entry), nil
	case "pause":
		if in.Name == "" {
			return "", errors.New("decime qué tarea pausar")
		}
		if err := store.Pause(ctx, userID, project, in.Name, true); err != nil {
			return "", err
		}
		return "pausada: " + in.Name, nil
	case "resume":
		if in.Name == "" {
			return "", errors.New("decime qué tarea despausar")
		}
		if err := store.Pause(ctx, userID, project, in.Name, false); err != nil {
			return "", err
		}
		return "en marcha: " + in.Name, nil
	case "remove":
		if in.Name == "" {
			return "", errors.New("decime qué tarea borrar")
		}
		if err := store.Remove(ctx, userID, project, in.Name); err != nil {
			return "", err
		}
		return "borrada: " + in.Name, nil
	}
	return "", fmt.Errorf("no conozco la acción %q, probá con add, list, show, pause, resume o remove", in.Action)
}

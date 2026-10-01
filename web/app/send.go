package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/3-lines-studio/goddard/axe"
)

// MaxSend is what the agent may put into a thread at once. It is the same
// ceiling the web upload has: an attachment of a conversation, either way in.
const MaxSend = 10 << 20

const sendSchema = `{
  "type": "object",
  "properties": {
    "path": {"type": "string", "description": "Path of the file to send, relative to the workspace or absolute"},
    "caption": {"type": "string", "description": "One line to show under it, if it needs saying"}
  },
  "required": ["path"]
}`

type sendArgs struct {
	Path    string `json:"path"`
	Caption string `json:"caption"`
}

// sendTool is how the agent shows a file: it takes one from the workspace,
// stores it with the attachments of the conversation and leaves a `file` event
// in the log, which is what the thread draws. The bytes are the same ones the
// upload route keeps, so the picture in the thread is the picture on disk.
func (s *Service) sendTool(conversationID string) axe.Tool {
	return axe.NewTool("send",
		"Send a file from the workspace to this conversation: a picture shows up in the thread, anything else as a link. Use it when a screenshot or a document is the answer, and not to show what you can describe in words.",
		sendSchema,
		func(in sendArgs) string {
			text, err := s.send(context.Background(), conversationID, in)
			if err != nil {
				return "error: " + err.Error()
			}
			return text
		})
}

func (s *Service) send(ctx context.Context, conversationID string, in sendArgs) (string, error) {
	if strings.TrimSpace(in.Path) == "" {
		return "", errors.New("decime qué archivo mandar")
	}
	info, err := s.Machine.Stat(in.Path)
	if err != nil {
		return "", err
	}
	if info.IsDir {
		return "", fmt.Errorf("%s es una carpeta y no un archivo", in.Path)
	}
	if info.Size > MaxSend {
		return "", fmt.Errorf("%s pesa %d bytes y el tope es %d", in.Path, info.Size, MaxSend)
	}
	bytes, err := s.Machine.Read(in.Path)
	if err != nil {
		return "", err
	}
	name := info.Name
	mime := http.DetectContentType(bytes)
	upload, err := s.Chat.PutUpload(ctx, conversationID, name, mime, bytes)
	if err != nil {
		return "", err
	}
	event := map[string]any{"event": "file", "id": upload.ID, "name": name, "mime": mime}
	if caption := strings.TrimSpace(in.Caption); caption != "" {
		event["caption"] = caption
	}
	if _, err := s.write(ctx, conversationID, event); err != nil {
		return "", err
	}
	return fmt.Sprintf("enviado %s (%d bytes)", name, len(bytes)), nil
}

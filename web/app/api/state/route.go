package state

import (
	"encoding/json"
	"net/http"

	"github.com/3-lines-studio/goddard/chat"
	"github.com/3-lines-studio/goddard/machine"
	"github.com/3-lines-studio/goddard/web/app"
)

type conversationView struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Source  string `json:"source"`
	Running bool   `json:"running"`
}

type projectView struct {
	ID            string             `json:"id"`
	Slug          string             `json:"slug"`
	Name          string             `json:"name"`
	Conversations []conversationView `json:"conversations"`
}

func Get(w http.ResponseWriter, r *http.Request) {
	service := app.Current()
	if service == nil {
		http.Error(w, "el servicio no arrancó", http.StatusServiceUnavailable)
		return
	}
	user, ok := app.Session(service, w, r)
	if !ok {
		return
	}
	orgs, err := service.OrgsOf(r.Context(), user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	projects, err := service.Chat.Projects(r.Context(), chat.Owner{Kind: chat.OwnerUser, ID: user.ID}, orgs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	views := make([]projectView, 0, len(projects))
	for _, project := range projects {
		conversations, err := service.Chat.Conversations(r.Context(), project.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		view := projectView{ID: project.ID, Slug: project.Slug, Name: project.Name, Conversations: []conversationView{}}
		for _, conversation := range conversations {
			if conversation.Source == chat.SourceSchedule {
				continue
			}
			view.Conversations = append(view.Conversations, conversationView{
				ID:      conversation.ID,
				Title:   conversation.Title,
				Source:  conversation.Source,
				Running: service.Running(conversation),
			})
		}
		views = append(views, view)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"projects":  views,
		"user":      user.Email,
		"workspace": service.Workspace,
		"machine":   machine.Usage(service.Workspace),
	})
}

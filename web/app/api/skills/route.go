package skills

import (
	"encoding/json"
	"net/http"

	"github.com/3-lines-studio/goddard/web/app"
)

type skillView struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Owner       string `json:"owner"`
	Role        string `json:"role"`
	Updated     int64  `json:"updated"`
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
	metas, err := service.Skill.List(r.Context(), service.Viewer(user))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	views := make([]skillView, 0, len(metas))
	for _, meta := range metas {
		views = append(views, skillView{
			Name:        meta.Name,
			Description: meta.Description,
			Owner:       meta.Owner.Kind,
			Role:        meta.Role,
			Updated:     meta.UpdatedAt,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"skills": views})
}

package skills

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/3-lines-studio/goddard/skill"
	"github.com/3-lines-studio/goddard/web/app/apptest"
)

func TestSkillsWantsASession(t *testing.T) {
	apptest.Route(t, apptest.Provider(t))
	recorder := httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/skills", nil, nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("sin sesión contestó %d", recorder.Code)
	}
}

func TestSkillsIsTheIndexThePromptShows(t *testing.T) {
	service := apptest.Route(t, apptest.Provider(t))
	cookie := apptest.Session(t, service, "berti@ejemplo.com")
	if err := service.Skill.Put(t.Context(), service.Viewer, skill.Skill{
		Meta: skill.Meta{
			Owner:       skill.Owner{Kind: skill.User, ID: service.Viewer.User},
			Name:        "picsel-deploy",
			Description: "Cómo se despliega picsel.",
		},
		Body: "# pasos",
	}); err != nil {
		t.Fatalf("no pude sembrar la skill: %v", err)
	}

	recorder := httptest.NewRecorder()
	Get(recorder, apptest.Request(t, "GET", "/api/skills", nil, cookie))
	if recorder.Code != http.StatusOK {
		t.Fatalf("contestó %d: %s", recorder.Code, apptest.Text(t, recorder))
	}
	var body struct {
		Skills []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Owner       string `json:"owner"`
		} `json:"skills"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("no pude leer la respuesta: %v", err)
	}
	if len(body.Skills) != 1 {
		t.Fatalf("llegaron %+v", body.Skills)
	}
	if body.Skills[0].Name != "picsel-deploy" || body.Skills[0].Owner != skill.User {
		t.Fatalf("la skill quedó %+v", body.Skills[0])
	}
}

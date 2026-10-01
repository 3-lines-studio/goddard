package skill

import "testing"

func TestTheToolListsAndLoads(t *testing.T) {
	store, viewer := testStore(t)
	put(t, store, viewer, Skill{
		Meta: Meta{Owner: Owner{Kind: Org, ID: "o1"}, Name: "browse", Description: "Navegar"},
		Body: "# browse\n\nUsá la tool.",
	})
	tool := Tool(store, viewer)
	if got := tool.Run(`{"action":"list"}`, nil).Text; got != "browse — Navegar" {
		t.Fatalf("list quedó %q", got)
	}
	if got := tool.Run(`{"action":"load","name":"browse"}`, nil).Text; got != "Skill browse\n\n# browse\n\nUsá la tool." {
		t.Fatalf("load quedó %q", got)
	}
}

func TestTheToolWritesWhatTheUserOwns(t *testing.T) {
	store, viewer := testStore(t)
	tool := Tool(store, viewer)
	got := tool.Run(`{"action":"save","name":"informe","description":"Cómo armar el informe","body":"# informe\n\nPaso uno."}`, nil).Text
	if got != "guardada la skill \"informe\" (20 bytes)" {
		t.Fatalf("save quedó %q", got)
	}
	if got := tool.Run(`{"action":"list"}`, nil).Text; got != "informe — Cómo armar el informe" {
		t.Fatalf("la lista quedó %q", got)
	}
	if got := tool.Run(`{"action":"load","name":"informe"}`, nil).Text; got != "Skill informe\n\n# informe\n\nPaso uno." {
		t.Fatalf("load quedó %q", got)
	}
	metas, err := store.List(t.Context(), viewer)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if metas[0].Owner != (Owner{Kind: User, ID: viewer.User}) {
		t.Fatalf("la skill quedó de %+v", metas[0].Owner)
	}
	if got := tool.Run(`{"action":"remove","name":"informe"}`, nil).Text; got != "borrada la skill \"informe\"" {
		t.Fatalf("remove quedó %q", got)
	}
	if got := tool.Run(`{"action":"list"}`, nil).Text; got != EmptyIndex {
		t.Fatalf("quedó algo: %q", got)
	}
	if got := tool.Run(`{"action":"remove","name":"informe"}`, nil).Text; got != `error: no hay una skill tuya que se llame "informe"` {
		t.Fatalf("borrar dos veces quedó %q", got)
	}
}

func TestTheToolTellsWhatWentWrong(t *testing.T) {
	store, viewer := testStore(t)
	tool := Tool(store, viewer)
	for _, raw := range []string{
		`{"action":"nope"}`,
		`{"action":"load"}`,
		`{"action":"load","name":"nope"}`,
		`{"action":"save","name":"sin-cuerpo","description":"algo"}`,
		`{"action":"save","name":"sin-descripcion","body":"algo"}`,
		`{"action":"save","description":"algo","body":"algo"}`,
		`{"action":"remove"}`,
	} {
		got := tool.Run(raw, nil).Text
		if len(got) < 7 || got[:7] != "error: " {
			t.Fatalf("%s quedó %q", raw, got)
		}
	}
}

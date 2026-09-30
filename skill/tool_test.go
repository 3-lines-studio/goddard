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

func TestTheToolTellsWhatWentWrong(t *testing.T) {
	store, viewer := testStore(t)
	tool := Tool(store, viewer)
	for _, raw := range []string{`{"action":"nope"}`, `{"action":"load"}`, `{"action":"load","name":"nope"}`} {
		got := tool.Run(raw, nil).Text
		if len(got) < 7 || got[:7] != "error: " {
			t.Fatalf("%s quedó %q", raw, got)
		}
	}
}

package skill

import (
	"errors"
	"testing"
)

func TestIndexIsOneNameAndDescriptionPerLine(t *testing.T) {
	metas := []Meta{
		{Name: "browse", Description: "Navegar"},
		{Name: "charts", Description: "Gráficos"},
	}
	want := "browse — Navegar\ncharts — Gráficos"
	if got := Index(metas); got != want {
		t.Fatalf("quedó %q", got)
	}
}

func TestIndexSaysSoWhenThereIsNothing(t *testing.T) {
	if got := Index(nil); got != EmptyIndex {
		t.Fatalf("quedó %q", got)
	}
}

func TestLoadIsAHeadingAndTheBody(t *testing.T) {
	got := Load(Skill{Meta: Meta{Name: "browse"}, Body: "# browse\n\nUsá la tool."})
	want := "Skill browse\n\n# browse\n\nUsá la tool."
	if got != want {
		t.Fatalf("quedó %q", got)
	}
}

func TestABadNameIsRejected(t *testing.T) {
	for _, name := range []string{"", "../secrets", "a/b", `a\b`} {
		if err := validateName(name); err == nil {
			t.Fatalf("%q pasó", name)
		}
	}
	if err := validateName("browse"); err != nil {
		t.Fatalf("browse no pasó: %v", err)
	}
}

func TestTheSystemIsReadOnly(t *testing.T) {
	owner := Owner{Kind: System}
	if err := canWrite(Viewer{Orgs: []string{"o1"}, User: "u1"}, owner); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("quedó %v", err)
	}
}

func TestOnlyTheOwnerWrites(t *testing.T) {
	viewer := Viewer{Orgs: []string{"o1"}, User: "u1"}
	if err := canWrite(viewer, Owner{Kind: Org, ID: "o1"}); err != nil {
		t.Fatalf("la propia organización no pasó: %v", err)
	}
	if err := canWrite(viewer, Owner{Kind: Org, ID: "o2"}); err == nil {
		t.Fatal("la organización ajena pasó")
	}
	if err := canWrite(viewer, Owner{Kind: User, ID: "u1"}); err != nil {
		t.Fatalf("el propio usuario no pasó: %v", err)
	}
	if err := canWrite(viewer, Owner{Kind: User, ID: "u2"}); err == nil {
		t.Fatal("el usuario ajeno pasó")
	}
	if err := canWrite(viewer, Owner{Kind: "otra-cosa"}); err == nil {
		t.Fatal("un dueño desconocido pasó")
	}
}

func TestClosestKeepsTheOwnersSkillAndSorts(t *testing.T) {
	metas := []Meta{
		{Owner: Owner{Kind: System}, Name: "browse", Description: "De fábrica"},
		{Owner: Owner{Kind: Org, ID: "o1"}, Name: "browse", Description: "De la org"},
		{Owner: Owner{Kind: User, ID: "u1"}, Name: "browse", Description: "Mía"},
		{Owner: Owner{Kind: Org, ID: "o1"}, Name: "agenda", Description: "De la org"},
	}
	got := closest(metas)
	if len(got) != 2 {
		t.Fatalf("quedaron %d", len(got))
	}
	if got[0].Name != "agenda" || got[1].Name != "browse" {
		t.Fatalf("el orden quedó %+v", got)
	}
	if got[1].Description != "Mía" {
		t.Fatalf("ganó %q", got[1].Description)
	}
}

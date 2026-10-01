package naming

import "testing"

func TestASlugIsWhatTheDirectoryAndTheOtherTablesHold(t *testing.T) {
	cases := map[string]string{
		"Goddard":              "goddard",
		"La web de Jimmy":      "la-web-de-jimmy",
		"Ñandú y café":         "nandu-y-cafe",
		"don't":                "dont",
		"  muchas   vueltas  ": "muchas-vueltas",
		"picsel-lab/2026":      "picsel-lab-2026",
	}
	for name, want := range cases {
		if got := From(name); got != want {
			t.Fatalf("%q dio %q, esperaba %q", name, got, want)
		}
	}
}

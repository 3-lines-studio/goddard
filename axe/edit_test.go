package axe

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func editOf(old, new string) editArg {
	return editArg{OldText: old, NewText: new}
}

func applyContent(t *testing.T, path, body string, edits ...editArg) string {
	t.Helper()
	result, err := applyEdits(path, body, edits)
	if err != nil {
		t.Fatalf("applyEdits: %v", err)
	}
	return result.content
}

func applyError(t *testing.T, path, body string, edits ...editArg) string {
	t.Helper()
	result, err := applyEdits(path, body, edits)
	if err == nil {
		t.Fatalf("esperaba error, salio: %q", result.content)
	}
	return err.Error()
}

func TestEditPreservesMixedLineEndings(t *testing.T) {
	body := "a\r\nb\nc\r\n"
	if got := applyContent(t, "f", body, editOf("a\r\n", "A\r\n")); got != "A\r\nb\nc\r\n" {
		t.Fatalf("crlf: %q", got)
	}
	if got := applyContent(t, "f", body, editOf("b", "B\nB2")); got != "a\r\nB\nB2\nc\r\n" {
		t.Fatalf("lf: %q", got)
	}
}

func TestEditCrlfRoundtrip(t *testing.T) {
	got := applyContent(t, "f", "one\r\ntwo\r\nthree\r\n", editOf("two", "TWO\nTWO2"))
	if got != "one\r\nTWO\r\nTWO2\r\nthree\r\n" {
		t.Fatalf("got: %q", got)
	}
}

func TestEditMultibyteContent(t *testing.T) {
	got := applyContent(t, "f", "héllo wörld 🙈\nsecond\n", editOf("wörld", "planet"))
	if got != "héllo planet 🙈\nsecond\n" {
		t.Fatalf("got: %q", got)
	}
}

func TestEditRejectsInvalidReplacements(t *testing.T) {
	if message := applyError(t, "f", "x x", editOf("x", "y")); !strings.Contains(message, "2 occurrences") {
		t.Errorf("duplicado: %q", message)
	}
	if message := applyError(t, "f", "abc", editOf("abc", "x"), editOf("bc", "y")); !strings.Contains(message, "overlap") {
		t.Errorf("solapado: %q", message)
	}
	if message := applyError(t, "f", "x", editOf("", "y")); !strings.Contains(message, "must not be empty") {
		t.Errorf("vacio: %q", message)
	}
	if message := applyError(t, "f", "x", editOf("x", "x")); !strings.Contains(message, "No changes made") {
		t.Errorf("sin cambio: %q", message)
	}
}

func TestEditPreservesBom(t *testing.T) {
	got := applyContent(t, "f", "\ufeffold", editOf("old", "new"))
	if got != "\ufeffnew" {
		t.Fatalf("got: %q", got)
	}
}

func TestEditFallsBackToFuzzyMatch(t *testing.T) {
	got := applyContent(t, "f", "let s = \u201chi\u201d;\n", editOf(`let s = "hi";`, `let s = "bye";`))
	if got != "let s = \"bye\";\n" {
		t.Fatalf("comillas: %q", got)
	}
	got = applyContent(t, "f", "a b   \nc\n", editOf("a b\nc\n", "X\n"))
	if got != "X\n" {
		t.Fatalf("espacios: %q", got)
	}
}

func TestEditFuzzyFoldsFullWidth(t *testing.T) {
	got := applyContent(t, "f", "let x = \uff081\uff09;\n", editOf("(1)", "(2)"))
	if got != "let x = (2);\n" {
		t.Fatalf("got: %q", got)
	}
}

func TestEditAcceptsQuirkInputs(t *testing.T) {
	cases := []string{
		`{"path":"f","edits":"[{\"oldText\":\"a\",\"newText\":\"b\"}]"}`,
		`{"path":"f","edits":{"oldText":"a","newText":"b"}}`,
		`{"path":"f","oldText":"a","newText":"b"}`,
	}
	for _, raw := range cases {
		var args editArgs
		if err := json.Unmarshal([]byte(raw), &args); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if len(args.Edits) != 1 || args.Path != "f" {
			t.Fatalf("%s: %+v", raw, args)
		}
	}

	for _, raw := range []string{`[]`, `"f"`, `null`} {
		var args editArgs
		if err := json.Unmarshal([]byte(raw), &args); err == nil {
			t.Fatalf("%s deberia fallar", raw)
		}
	}
	if err := json.Unmarshal([]byte(`{"edits":[]}`), &editArgs{}); err == nil {
		t.Fatal("sin path deberia fallar")
	}
}

func TestEditRunsThroughTheToolSchema(t *testing.T) {
	fake := newFakeMachine()
	fake.files["/a.txt"] = []byte("hola\nmundo\n")
	tool := EditTool(fake)

	out := tool.Run(`{"path":"/a.txt","edits":"[{\"oldText\":\"hola\",\"newText\":\"chau\"}]"}`, nil).Text
	if !strings.Contains(out, "Successfully replaced 1 block(s)") {
		t.Fatalf("got: %q", out)
	}
	if string(fake.files["/a.txt"]) != "chau\nmundo\n" {
		t.Fatalf("guardado: %q", fake.files["/a.txt"])
	}

	out = tool.Run(`{"path":"/a.txt","edits":[]}`, nil).Text
	if out != "error: edits must contain at least one replacement" {
		t.Fatalf("got: %q", out)
	}
}

func TestEditReturnsUnifiedPatch(t *testing.T) {
	result, err := applyEdits("f.txt", "a\nb\nc\n", []editArg{editOf("b\n", "B\n")})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"@@ -1,3 +1,3 @@", "-b", "+B"} {
		if !strings.Contains(result.patch, want) {
			t.Fatalf("falta %q en:\n%s", want, result.patch)
		}
	}
}

func TestEditTruncatesLargePatch(t *testing.T) {
	var old, replacement strings.Builder
	for i := range 500 {
		fmt.Fprintf(&old, "old line %d\n", i)
		fmt.Fprintf(&replacement, "new line %d\n", i)
	}
	result, err := applyEdits("f", old.String(), []editArg{editOf(old.String(), replacement.String())})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.patch, "lines omitted") {
		t.Fatalf("sin elision:\n%s", result.patch)
	}
	if lines := len(splitLines(result.patch)); lines > 82 {
		t.Fatalf("%d lineas de patch", lines)
	}
}

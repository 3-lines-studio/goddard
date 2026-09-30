package axe

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

func readTestdata(t *testing.T, path string) map[string]string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("testdata %s: %v", path, err)
	}
	defer file.Close()
	expected := map[string]string{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		name, value, found := strings.Cut(scanner.Text(), "\t")
		if !found {
			continue
		}
		expected[name] = value
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("testdata %s: %v", path, err)
	}
	return expected
}

func escapeEdit(s string) string {
	return strings.NewReplacer(
		`\`, `\\`,
		"\n", `\n`,
		"\r", `\r`,
		"\t", `\t`,
	).Replace(s)
}

type editCase struct {
	body  string
	edits []editArg
}

func paridadEditCases() map[string]editCase {
	return map[string]editCase{
		"mixed_crlf":          {"a\r\nb\nc\r\n", []editArg{editOf("a\r\n", "A\r\n")}},
		"mixed_lf":            {"a\r\nb\nc\r\n", []editArg{editOf("b", "B\nB2")}},
		"crlf_roundtrip":      {"one\r\ntwo\r\nthree\r\n", []editArg{editOf("two", "TWO\nTWO2")}},
		"crlf_all":            {"one\r\ntwo\r\n", []editArg{editOf("two", "TWO")}},
		"multibyte":           {"héllo wörld 🙈\nsecond\n", []editArg{editOf("wörld", "planet")}},
		"bom":                 {"\ufeffold", []editArg{editOf("old", "new")}},
		"duplicate":           {"x x", []editArg{editOf("x", "y")}},
		"overlap":             {"abc", []editArg{editOf("abc", "x"), editOf("bc", "y")}},
		"empty_old":           {"x", []editArg{editOf("", "y")}},
		"no_change":           {"x", []editArg{editOf("x", "x")}},
		"not_found":           {"hola", []editArg{editOf("chau", "x")}},
		"not_found_multi":     {"hola", []editArg{editOf("hola", "x"), editOf("chau", "y")}},
		"fuzzy_quotes":        {"let s = \u201chi\u201d;\n", []editArg{editOf(`let s = "hi";`, `let s = "bye";`)}},
		"fuzzy_trailing":      {"a b   \nc\n", []editArg{editOf("a b\nc\n", "X\n")}},
		"fullwidth":           {"let x = \uff081\uff09;\n", []editArg{editOf("(1)", "(2)")}},
		"fuzzy_dash":          {"a \u2014 b\n", []editArg{editOf("a - b", "c")}},
		"no_final_newline":    {"a\nb", []editArg{editOf("b", "B")}},
		"append_line":         {"a\n", []editArg{editOf("a\n", "a\nb\n")}},
		"delete_text":         {"a\nb\nc\n", []editArg{editOf("b\n", "")}},
		"two_edits":           {"a\nb\nc\nd\n", []editArg{editOf("b", "B"), editOf("d", "D")}},
		"two_edits_one_group": {"a\nb\nc\n", []editArg{editOf("a", "A"), editOf("c", "C")}},
		"edit_inside_line":    {"x = 1; y = 2;\n", []editArg{editOf("1", "11")}},
		"fuzzy_crlf":          {"a\r\nb\r\n", []editArg{editOf("b", "B")}},
		"empty_file":          {"", []editArg{editOf("a", "b")}},
		"only_newline":        {"\n", []editArg{editOf("\n", "x\n")}},
		"repeat_context":      {"same\nother\nsame\n", []editArg{editOf("other", "OTHER")}},
	}
}

func paridadEditCasesForTest() map[string]func() string {
	cases := map[string]func() string{}
	for name, test := range paridadEditCases() {
		run := func() (applied, error) { return applyEdits("f.txt", test.body, test.edits) }
		cases[name] = func() string {
			result, err := run()
			if err != nil {
				return "ERROR\t" + escapeEdit(err.Error())
			}
			return escapeEdit(result.content)
		}
		cases[name+".patch"] = func() string {
			result, err := run()
			if err != nil {
				return "ERROR\t" + escapeEdit(err.Error())
			}
			return escapeEdit(result.patch)
		}
	}
	return cases
}

func TestParidadEditConRust(t *testing.T) {
	expected := readTestdata(t, "testdata/paridad-edit.txt")
	cases := paridadEditCasesForTest()
	for name := range expected {
		if _, covered := cases[name]; !covered {
			t.Errorf("el dump trae el caso %q y nadie lo cubre", name)
		}
	}
	for name, want := range expected {
		check, covered := cases[name]
		if !covered {
			continue
		}
		if got := check(); got != want {
			t.Errorf("%s:\n got:  %s\n want: %s", name, got, want)
		}
	}
}

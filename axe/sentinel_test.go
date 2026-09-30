package axe

import (
	"testing"
)

type sentinelCase struct {
	text   string
	values []string
}

func sentinelCases() map[string]sentinelCase {
	return map[string]sentinelCase{
		"known_value":            {"key is hunter2abc ok", []string{"hunter2abc"}},
		"known_value_absent":     {"key is zzz ok", []string{"hunter2abc"}},
		"known_longest_first":    {"x abcdefgh y", []string{"abcdef", "abcdefgh"}},
		"known_short_skipped":    {"ab", []string{"ab"}},
		"known_twice":            {"a hunter2abc b hunter2abc c", []string{"hunter2abc"}},
		"pem_block":              {"a\n-----BEGIN PRIVATE KEY-----\nMIIB\n-----END PRIVATE KEY-----\nb", nil},
		"pem_truncated":          {"a\n-----BEGIN PRIVATE KEY-----\nMIIB", nil},
		"url_basic_auth":         {"postgres://user:pass@localhost:5432/db", nil},
		"url_no_colon":           {"see https://example.com/a:b@c", nil},
		"url_plain":              {"see https://example.com/a", nil},
		"url_user_only":          {"https://user@host/x", nil},
		"token_sk":               {"tok sk-abcdefghijklmnopqrstuvwx end", nil},
		"token_aws":              {"aws AKIAIOSFODNN7EXAMPLE here", nil},
		"token_github":           {"gh ghp_abcdefghijklmnopqrstuvwxyz012345 here", nil},
		"token_short":            {"sk-abcdef", nil},
		"token_inside_word":      {"xsk-abcdefghijklmnopqrstuvwx", nil},
		"jwt":                    {"jwt eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.abcdefghijklmnopqrstuvwxyz012345", nil},
		"jwt_short":              {"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0", nil},
		"assign_password":        {"PASSWORD=hunter2 horse", nil},
		"assign_json":            {`{"api_key": "abc123", "path": "/x"}`, nil},
		"assign_flag":            {"run --token abc123 --verbose", nil},
		"assign_authorization":   {"Authorization: Bearer abc123", nil},
		"assign_basic":           {"Authorization: Basic dXNlcjpwYXNz", nil},
		"assign_prose":           {"the password is required", nil},
		"assign_space_separated": {"machine x login alice password s3cr3t", nil},
		"assign_empty":           {"TRANSCRIBE_API_KEY=\nnext line", nil},
		"assign_lowercase_key":   {"api_key=abc123 ok", nil},
		"assign_camel":           {"clientSecret: abc123 ok", nil},
		"assign_quoted":          {"token = 'abc123' ok", nil},
		"assign_already":         {"PASSWORD=[REDACTED] x", nil},
		"ordinary":               {"let content = compute();\npath=/tmp/file\nkeyboard=us", nil},
		"empty":                  {"", nil},
	}
}

func sentinelRun(test sentinelCase) *Sentinel {
	sentinel := NewSentinel()
	for _, value := range test.values {
		sentinel.AddValue(value)
	}
	return sentinel
}

func TestParidadSentinelConRust(t *testing.T) {
	expected := readTestdata(t, "testdata/paridad-sentinel.txt")
	cases := map[string]func() string{}
	for name, test := range sentinelCases() {
		cases[name] = func() string { return escapeEdit(sentinelRun(test).Redact(test.text)) }
	}
	cases["idempotent"] = func() string {
		sentinel := NewSentinel()
		once := sentinel.Redact("PASSWORD=hunter2 x")
		return escapeEdit(sentinel.Redact(once))
	}
	cases["idempotent_once"] = func() string {
		return escapeEdit(NewSentinel().Redact("PASSWORD=hunter2 x"))
	}
	cases["message_user"] = func() string {
		sentinel := NewSentinel()
		sentinel.AddValue("hunter2abc")
		return escapeEdit(sentinel.RedactMessage(Message{Role: "user", Content: "my key is hunter2abc"}).Content)
	}
	cases["message_assistant"] = func() string {
		sentinel := NewSentinel()
		sentinel.AddValue("hunter2abc")
		return escapeEdit(sentinel.RedactMessage(Message{Role: "assistant", Content: "my key is hunter2abc"}).Content)
	}

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

func TestSentinelAddValueRules(t *testing.T) {
	sentinel := NewSentinel()
	sentinel.AddValue("abc")
	if len(sentinel.values) != 0 {
		t.Fatal("un valor corto se coló")
	}
	sentinel.AddValue("abcdef")
	sentinel.AddValue("abcdef")
	if len(sentinel.values) != 1 {
		t.Fatalf("duplicado: %v", sentinel.values)
	}
	sentinel.AddValue("abcdefgh")
	if sentinel.values[0] != "abcdefgh" {
		t.Fatalf("orden: %v", sentinel.values)
	}
}

func TestSentinelFromEnvHidesTokens(t *testing.T) {
	t.Setenv("AXE_TEST_SECRET_TOKEN", "abcdef123456")
	sentinel := NewSentinel().FromEnv()
	if !contains(sentinel.values, "abcdef123456") {
		t.Fatalf("no tomo el token del entorno: %v", sentinel.values)
	}
	t.Setenv("AXE_TEST_PLAIN_PATH", "/usr/local/bin")
	sentinel = NewSentinel().FromEnv()
	if contains(sentinel.values, "/usr/local/bin") {
		t.Fatal("tomo un path como secreto")
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

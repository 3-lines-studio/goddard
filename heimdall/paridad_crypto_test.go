package heimdall

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
			t.Fatalf("testdata %s: línea sin tabulador: %q", path, scanner.Text())
		}
		expected[name] = value
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("testdata %s: %v", path, err)
	}
	return expected
}

func TestParidadCriptoConRust(t *testing.T) {
	expected := readTestdata(t, "testdata/paridad-crypto.txt")
	key := testKey(t)
	derive := func(context string) string {
		derived := key.Derive(context)
		return EncodeHex(derived.material[:])
	}
	seal := func(name, plain, aad string) func() string {
		return func() string {
			blob, err := DecodeHex(expected["seal_"+name])
			if err != nil {
				t.Fatalf("seal_%s: %v", name, err)
			}
			nonce := blob[:nonceSize]
			derived := key.Derive("secrets/bifrost/dev")
			return EncodeHex(derived.sealWith(nonce, []byte(plain), []byte(aad)))
		}
	}
	cases := map[string]func() string{
		"derive_bifrost_dev":  func() string { return derive("secrets/bifrost/dev") },
		"derive_bifrost_prod": func() string { return derive("secrets/bifrost/prod") },
		"derive_wildcard":     func() string { return derive("secrets/*/dev") },
		"derive_empty":        func() string { return derive("") },

		"hash_hd_hola":        func() string { return Hash("hd_hola") },
		"hash_empty":          func() string { return Hash("") },
		"hash_twentyfour_hex": func() string { return Hash(strings.Repeat("ab", 24)) },

		"seal_stripe":      seal("stripe", "sk_test_123", "secrets/bifrost/dev/STRIPE_KEY"),
		"seal_password":    seal("password", "hunter2", "secrets/bifrost/dev/DB_PASSWORD"),
		"seal_empty_value": seal("empty_value", "", "secrets/bifrost/dev/EMPTY"),
		"seal_empty_aad":   seal("empty_aad", "sin aad", ""),
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

func TestAbreLoQueSelloElRust(t *testing.T) {
	expected := readTestdata(t, "testdata/paridad-crypto.txt")
	key := testKey(t).Derive("secrets/bifrost/dev")
	cases := []struct {
		name  string
		aad   string
		plain string
	}{
		{"seal_stripe", "secrets/bifrost/dev/STRIPE_KEY", "sk_test_123"},
		{"seal_password", "secrets/bifrost/dev/DB_PASSWORD", "hunter2"},
		{"seal_empty_value", "secrets/bifrost/dev/EMPTY", ""},
		{"seal_empty_aad", "", "sin aad"},
	}
	for _, caso := range cases {
		blob, err := DecodeHex(expected[caso.name])
		if err != nil {
			t.Fatalf("%s: %v", caso.name, err)
		}
		plain, err := key.Open(blob, []byte(caso.aad))
		if err != nil {
			t.Errorf("%s: %v", caso.name, err)
			continue
		}
		if string(plain) != caso.plain {
			t.Errorf("%s: abrió %q", caso.name, plain)
		}
	}
}

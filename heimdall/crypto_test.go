package heimdall

import (
	"bytes"
	"strings"
	"testing"
)

func testKey(t *testing.T) Key {
	t.Helper()
	key, err := KeyFromHex(strings.Repeat("ab", 32))
	if err != nil {
		t.Fatalf("master key: %v", err)
	}
	return key
}

func TestSealAndOpenRoundTrip(t *testing.T) {
	key := testKey(t)
	blob, err := key.Seal([]byte("secreto"), []byte("donde"))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	plain, err := key.Open(blob, []byte("donde"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if string(plain) != "secreto" {
		t.Fatalf("abrió %q", plain)
	}
	if bytes.Contains(blob, []byte("secreto")) {
		t.Fatal("el texto plano quedó adentro del sobre")
	}
}

func TestEachSealUsesAFreshNonce(t *testing.T) {
	key := testKey(t)
	first, err := key.Seal([]byte("x"), []byte("a"))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	second, err := key.Seal([]byte("x"), []byte("a"))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if bytes.Equal(first, second) {
		t.Fatal("los dos sobres salieron iguales")
	}
}

func TestAnotherKeyDoesNotOpen(t *testing.T) {
	key := testKey(t)
	blob, err := key.Seal([]byte("secreto"), []byte("donde"))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	other, err := KeyFromHex(strings.Repeat("cd", 32))
	if err != nil {
		t.Fatalf("master key: %v", err)
	}
	if _, err := other.Open(blob, []byte("donde")); err == nil {
		t.Fatal("otra clave abrió el sobre")
	}
}

func TestAContextDoesNotOpenAnother(t *testing.T) {
	key := testKey(t)
	blob, err := key.Derive("proyecto/a").Seal([]byte("x"), []byte("donde"))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	plain, err := key.Derive("proyecto/a").Open(blob, []byte("donde"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if string(plain) != "x" {
		t.Fatalf("abrió %q", plain)
	}
	if _, err := key.Derive("proyecto/b").Open(blob, []byte("donde")); err == nil {
		t.Fatal("otro contexto abrió el sobre")
	}
}

func TestABlobDoesNotOpenUnderAnotherName(t *testing.T) {
	key := testKey(t)
	blob, err := key.Seal([]byte("secreto"), []byte("bifrost/dev/ALFA"))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	plain, err := key.Open(blob, []byte("bifrost/dev/ALFA"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if string(plain) != "secreto" {
		t.Fatalf("abrió %q", plain)
	}
	if _, err := key.Open(blob, []byte("bifrost/dev/BETA")); err == nil {
		t.Fatal("abrió con otro nombre")
	}
	if _, err := key.Open(blob, nil); err == nil {
		t.Fatal("abrió sin nombre")
	}
}

func TestTruncatedBlobsDoNotOpen(t *testing.T) {
	if _, err := testKey(t).Open([]byte("corto"), []byte("a")); err == nil {
		t.Fatal("abrió un sobre cortado")
	}
}

func TestMasterKeyIs32BytesOfHex(t *testing.T) {
	if _, err := KeyFromHex("ab"); err == nil {
		t.Fatal("aceptó una master key de un byte")
	}
	if _, err := KeyFromHex(strings.Repeat("zz", 32)); err == nil {
		t.Fatal("aceptó algo que no es hexadecimal")
	}
	if _, err := KeyFromHex(strings.Repeat("ab", 32)); err != nil {
		t.Fatalf("rechazó la master key buena: %v", err)
	}
}

func TestHexRoundTrips(t *testing.T) {
	got, err := DecodeHex(EncodeHex([]byte{0, 15, 255}))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !bytes.Equal(got, []byte{0, 15, 255}) {
		t.Fatalf("ida y vuelta dio %v", got)
	}
}

func TestEqualIsLengthAware(t *testing.T) {
	if !Equal("abc", "abc") {
		t.Fatal("dos iguales no dieron igual")
	}
	if Equal("abc", "abd") {
		t.Fatal("dos distintos dieron igual")
	}
	if Equal("abc", "ab") {
		t.Fatal("dos de distinto largo dieron igual")
	}
}

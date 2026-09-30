package heimdall

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/zeebo/blake3"
	"golang.org/x/crypto/chacha20poly1305"
)

const nonceSize = chacha20poly1305.NonceSizeX

type Key struct {
	material [32]byte
}

func KeyFromHex(text string) (Key, error) {
	bytes, err := DecodeHex(text)
	if err != nil {
		return Key{}, err
	}
	if len(bytes) != 32 {
		return Key{}, fmt.Errorf("la master key tiene %d bytes, espera 32", len(bytes))
	}
	var key Key
	copy(key.material[:], bytes)
	return key, nil
}

func (k Key) Derive(context string) Key {
	derived := blake3.NewDeriveKey(context)
	derived.Write(k.material[:])
	var key Key
	derived.Sum(key.material[:0])
	return key
}

func (k Key) Seal(plain, aad []byte) ([]byte, error) {
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("no hay azar: %v", err)
	}
	return k.sealWith(nonce, plain, aad), nil
}

func (k Key) sealWith(nonce, plain, aad []byte) []byte {
	cipher, _ := chacha20poly1305.NewX(k.material[:])
	sealed := cipher.Seal(nil, nonce, plain, aad)
	blob := make([]byte, 0, nonceSize+len(sealed))
	blob = append(blob, nonce...)
	return append(blob, sealed...)
}

func (k Key) Open(blob, aad []byte) ([]byte, error) {
	if len(blob) <= nonceSize {
		return nil, errors.New("el archivo cifrado está cortado")
	}
	cipher, _ := chacha20poly1305.NewX(k.material[:])
	plain, err := cipher.Open(nil, blob[:nonceSize], blob[nonceSize:], aad)
	if err != nil {
		return nil, errors.New("no pude descifrar: ¿cambió la master key?")
	}
	return plain, nil
}

func RandomHex(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("no hay azar: %v", err)
	}
	return EncodeHex(buf), nil
}

func Hash(text string) string {
	sum := blake3.Sum256([]byte(text))
	return EncodeHex(sum[:])
}

func Equal(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := 0; i < len(a); i++ {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}

func EncodeHex(bytes []byte) string {
	return hex.EncodeToString(bytes)
}

func DecodeHex(text string) ([]byte, error) {
	text = strings.TrimSpace(text)
	if len(text)%2 != 0 {
		return nil, errors.New("el hexadecimal tiene largo impar")
	}
	bytes, err := hex.DecodeString(text)
	if err != nil {
		return nil, errors.New("no es hexadecimal")
	}
	return bytes, nil
}

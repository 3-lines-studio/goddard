package compute

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"strings"

	"golang.org/x/crypto/ssh"
)

// NewKeyPair is a pair for a machine of the house: the public half is left on
// the machine and the private one is kept in heimdall, which is what makes a
// sandbox goddard made for an owner the same thing as one the owner brought.
func NewKeyPair() (string, []byte, error) {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", nil, err
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		return "", nil, err
	}
	block, err := ssh.MarshalPrivateKey(private, "")
	if err != nil {
		return "", nil, err
	}
	public := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))
	return public, pem.EncodeToMemory(block), nil
}

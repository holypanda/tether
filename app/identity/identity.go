package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

type Identity struct {
	priv ed25519.PrivateKey
	pub  ed25519.PublicKey
}

func GenerateOrLoad(path string) (*Identity, error) {
	if data, err := os.ReadFile(path); err == nil {
		block, _ := pem.Decode(data)
		if block == nil {
			return nil, os.ErrInvalid
		}
		priv := ed25519.PrivateKey(block.Bytes)
		return &Identity{priv: priv, pub: priv.Public().(ed25519.PublicKey)}, nil
	}
	return generateAndSave(path)
}

func Ephemeral() (*Identity, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &Identity{priv: priv, pub: pub}, nil
}

func generateAndSave(path string) (*Identity, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	block := &pem.Block{Type: "ED25519 PRIVATE KEY", Bytes: priv}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		return nil, err
	}
	return &Identity{priv: priv, pub: pub}, nil
}

func (i *Identity) Signer() (ssh.Signer, error) {
	return ssh.NewSignerFromKey(i.priv)
}

func (i *Identity) PublicKey() ssh.PublicKey {
	pk, _ := ssh.NewPublicKey(i.pub)
	return pk
}

func (i *Identity) AuthorizedKey() string {
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(i.PublicKey())))
}

func (i *Identity) PrivatePEM() []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "ED25519 PRIVATE KEY", Bytes: i.priv})
}

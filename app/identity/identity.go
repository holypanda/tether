package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

type Identity struct {
	priv ed25519.PrivateKey
	pub  ed25519.PublicKey
}

// GenerateOrLoad loads an existing Ed25519 key from path, or generates and saves a new one
// if the file does not exist. Any other read/parse error is returned to the caller rather
// than silently regenerating.
//
// On Windows, the key file's NTFS ACL is locked down to the current user on every load
// (idempotent), because OpenSSH ssh.exe refuses to use keys with inherited/overly-permissive
// ACLs and silently falls back to password prompt.
func GenerateOrLoad(path string) (*Identity, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("identity: read key %s: %w", path, err)
		}
		return generateAndSave(path)
	}
	key, err := ssh.ParseRawPrivateKey(data)
	if err != nil {
		return nil, fmt.Errorf("identity: parse key %s: %w", path, err)
	}
	privPtr, ok := key.(*ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("identity: expected *ed25519.PrivateKey, got %T", key)
	}
	priv := *privPtr
	if len(priv) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("identity: unexpected key length %d", len(priv))
	}
	// Best-effort ACL fix on every load — handles keys created before the lockdown
	// was in place, or keys moved between machines.
	_ = lockdownKeyFile(path)
	return &Identity{priv: priv, pub: priv.Public().(ed25519.PublicKey)}, nil
}

// Ephemeral generates an in-memory Ed25519 key pair without persisting it.
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
	pemBlock, err := ssh.MarshalPrivateKey(priv, "stim-link")
	if err != nil {
		return nil, fmt.Errorf("identity: marshal openssh: %w", err)
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(pemBlock), 0o600); err != nil {
		return nil, err
	}
	// On Windows, NTFS ACLs override the 0o600 mode above. Lock down explicitly.
	if err := lockdownKeyFile(path); err != nil {
		return nil, fmt.Errorf("identity: lock down key file: %w", err)
	}
	return &Identity{priv: priv, pub: pub}, nil
}

func (i *Identity) Signer() (ssh.Signer, error) {
	return ssh.NewSignerFromKey(i.priv)
}

// PublicKey returns the SSH-wire-format public key. Panics on the impossible case
// where ssh.NewPublicKey rejects a valid Ed25519 public key, which would indicate
// a library-level bug.
func (i *Identity) PublicKey() ssh.PublicKey {
	pk, err := ssh.NewPublicKey(i.pub)
	if err != nil {
		panic(fmt.Sprintf("identity: ssh.NewPublicKey rejected Ed25519 key: %v", err))
	}
	return pk
}

// AuthorizedKey returns the one-line authorized_keys format, no trailing newline.
func (i *Identity) AuthorizedKey() string {
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(i.PublicKey())))
}

// PrivatePEM returns the private key in standard OpenSSH PEM format, suitable for
// writing to disk and use with the OpenSSH command-line ssh / sshfs.
func (i *Identity) PrivatePEM() []byte {
	block, err := ssh.MarshalPrivateKey(i.priv, "stim-link")
	if err != nil {
		panic(fmt.Sprintf("identity: marshal private key: %v", err))
	}
	return pem.EncodeToMemory(block)
}

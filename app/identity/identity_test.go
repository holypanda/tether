package identity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestGenerateAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test_ed25519")

	id1, err := GenerateOrLoad(path)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.HasPrefix(id1.AuthorizedKey(), "ssh-ed25519 ") {
		t.Errorf("authorized_keys format wrong: %q", id1.AuthorizedKey())
	}

	id2, err := GenerateOrLoad(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if id1.AuthorizedKey() != id2.AuthorizedKey() {
		t.Errorf("second load should return same key")
	}
}

func TestEphemeral(t *testing.T) {
	id, err := Ephemeral()
	if err != nil {
		t.Fatalf("ephemeral: %v", err)
	}
	if len(id.PrivatePEM()) == 0 {
		t.Error("ephemeral private key empty")
	}
}

func TestLoadCorruptedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt_ed25519")

	// Write garbage
	if err := os.WriteFile(path, []byte("not a valid pem"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := GenerateOrLoad(path); err == nil {
		t.Error("expected error loading corrupted key file")
	}
}

func TestPrivatePEMIsOpenSSHFormat(t *testing.T) {
	id, err := Ephemeral()
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := id.PrivatePEM()
	if !strings.Contains(string(pemBytes), "OPENSSH PRIVATE KEY") {
		t.Errorf("expected OPENSSH PRIVATE KEY PEM block, got:\n%s", pemBytes)
	}
	// Verify it round-trips through ssh.ParseRawPrivateKey
	if _, err := ssh.ParseRawPrivateKey(pemBytes); err != nil {
		t.Errorf("ssh.ParseRawPrivateKey rejected our output: %v", err)
	}
}

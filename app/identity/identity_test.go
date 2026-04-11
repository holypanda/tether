package identity

import (
	"path/filepath"
	"strings"
	"testing"
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

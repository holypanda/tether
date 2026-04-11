package sftpserver

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"stim-link/identity"
)

func TestSFTPServerRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}

	serverID, _ := identity.Ephemeral()
	clientID, _ := identity.Ephemeral()

	srv, err := Start(Config{
		RootDir:          dir,
		HostIdentity:     serverID,
		AllowedClientKey: clientID.PublicKey(),
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Close()

	clientSigner, _ := clientID.Signer()
	cfg := &ssh.ClientConfig{
		User:            "winshare",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(clientSigner)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	conn, err := ssh.Dial("tcp", srv.Addr(), cfg)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	sc, err := sftp.NewClient(conn)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer sc.Close()

	f, err := sc.Open("hello.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()
	content, _ := io.ReadAll(f)
	if string(content) != "world" {
		t.Errorf("got %q, want %q", content, "world")
	}
}

func TestSFTPServerRejectsWrongKey(t *testing.T) {
	dir := t.TempDir()
	serverID, _ := identity.Ephemeral()
	allowedID, _ := identity.Ephemeral()
	wrongID, _ := identity.Ephemeral()

	srv, err := Start(Config{
		RootDir:          dir,
		HostIdentity:     serverID,
		AllowedClientKey: allowedID.PublicKey(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	wrongSigner, _ := wrongID.Signer()
	cfg := &ssh.ClientConfig{
		User:            "winshare",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(wrongSigner)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	_, err = ssh.Dial("tcp", srv.Addr(), cfg)
	if err == nil {
		t.Error("expected auth failure")
	}
}

func TestSFTPServerRejectsEscape(t *testing.T) {
	dir := t.TempDir()
	serverID, _ := identity.Ephemeral()
	clientID, _ := identity.Ephemeral()

	srv, _ := Start(Config{
		RootDir:          dir,
		HostIdentity:     serverID,
		AllowedClientKey: clientID.PublicKey(),
	})
	defer srv.Close()

	signer, _ := clientID.Signer()
	conn, _ := ssh.Dial("tcp", srv.Addr(), &ssh.ClientConfig{
		User:            "winshare",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	defer conn.Close()
	sc, _ := sftp.NewClient(conn)
	defer sc.Close()

	// Try to read a file outside the root
	_, err := sc.Open("/etc/passwd")
	if err == nil {
		t.Error("expected permission denied when escaping root")
	}
}

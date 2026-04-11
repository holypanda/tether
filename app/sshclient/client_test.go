package sshclient

import (
	"fmt"
	"net"
	"testing"

	"golang.org/x/crypto/ssh"

	"stim-link/identity"
)

// startTestSSHServer starts an in-process SSH server on 127.0.0.1 that accepts
// either a fixed password or an allowed public key. Returns the address and a
// cleanup function.
func startTestSSHServer(t *testing.T, password string, allowed ssh.PublicKey) (string, func()) {
	t.Helper()
	hostID, _ := identity.Ephemeral()
	hostSigner, _ := hostID.Signer()

	cfg := &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
			if string(pw) == password {
				return &ssh.Permissions{}, nil
			}
			return nil, ssh.ErrNoAuth
		},
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if allowed != nil && string(key.Marshal()) == string(allowed.Marshal()) {
				return &ssh.Permissions{}, nil
			}
			return nil, ssh.ErrNoAuth
		},
	}
	cfg.AddHostKey(hostSigner)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				_, chans, reqs, err := ssh.NewServerConn(conn, cfg)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(reqs)
				for newCh := range chans {
					_ = newCh.Reject(ssh.UnknownChannelType, "no sessions in test")
				}
			}()
		}
	}()

	return ln.Addr().String(), func() { ln.Close() }
}

func splitHostPort(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	var p int
	if _, err := fmt.Sscanf(portStr, "%d", &p); err != nil {
		t.Fatal(err)
	}
	return host, p
}

func TestDialWithPassword(t *testing.T) {
	addr, stop := startTestSSHServer(t, "secret", nil)
	defer stop()
	host, port := splitHostPort(t, addr)

	c, err := DialPassword(host, port, "testuser", "secret")
	if err != nil {
		t.Fatalf("DialPassword: %v", err)
	}
	defer c.Close()
}

func TestDialWithKey(t *testing.T) {
	clientID, _ := identity.Ephemeral()
	addr, stop := startTestSSHServer(t, "", clientID.PublicKey())
	defer stop()
	host, port := splitHostPort(t, addr)

	c, err := DialKey(host, port, "testuser", clientID)
	if err != nil {
		t.Fatalf("DialKey: %v", err)
	}
	defer c.Close()
}

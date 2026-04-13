package sshclient

import (
	"fmt"
	"net"
	"testing"

	"golang.org/x/crypto/ssh"

	"tether/identity"
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

func startTestSSHServerWithExec(t *testing.T, password string) (string, func()) {
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
	}
	cfg.AddHostKey(hostSigner)

	ln, _ := net.Listen("tcp", "127.0.0.1:0")
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
					if newCh.ChannelType() != "session" {
						_ = newCh.Reject(ssh.UnknownChannelType, "")
						continue
					}
					ch, reqs, _ := newCh.Accept()
					go func(ch ssh.Channel, reqs <-chan *ssh.Request) {
						defer ch.Close()
						for req := range reqs {
							if req.Type == "exec" {
								_ = req.Reply(true, nil)
								ch.Write([]byte("OK\n"))
								ch.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
								return
							}
							_ = req.Reply(false, nil)
						}
					}(ch, reqs)
				}
			}()
		}
	}()
	return ln.Addr().String(), func() { ln.Close() }
}

func TestRunCommand(t *testing.T) {
	addr, stop := startTestSSHServerWithExec(t, "pw")
	defer stop()
	host, port := splitHostPort(t, addr)

	c, err := DialPassword(host, port, "u", "pw")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	out, err := c.Run("echo hello")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if string(out) != "OK\n" {
		t.Errorf("got %q", out)
	}
}

func TestRunScript(t *testing.T) {
	addr, stop := startTestSSHServerWithExec(t, "pw")
	defer stop()
	host, port := splitHostPort(t, addr)

	c, err := DialPassword(host, port, "u", "pw")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	out, err := c.RunScript("echo hello\necho world\n")
	if err != nil {
		t.Fatalf("RunScript: %v", err)
	}
	if string(out) != "OK\n" {
		t.Errorf("got %q", out)
	}
}

func TestReverseForwardProxiesEndToEnd(t *testing.T) {
	// This test uses a real ssh server (the one started by startTestSSHServer)
	// but that server doesn't implement tcpip-forward requests. Skip for now —
	// real reverse forward is covered by E2E testing against VPS.
	t.Skip("reverse forward requires real sshd; covered by manual E2E")
}

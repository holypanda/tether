package sshclient

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"stim-link/identity"
)

type Client struct {
	conn *ssh.Client
}

func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *Client) Raw() *ssh.Client { return c.conn }

func DialPassword(host string, port int, user, password string) (*Client, error) {
	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}
	return dial(host, port, cfg)
}

func DialKey(host string, port int, user string, id *identity.Identity) (*Client, error) {
	signer, err := id.Signer()
	if err != nil {
		return nil, err
	}
	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}
	return dial(host, port, cfg)
}

func dial(host string, port int, cfg *ssh.ClientConfig) (*Client, error) {
	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn}, nil
}

// ReverseForward asks the remote host to listen on remoteAddr (e.g. "localhost:2222").
// Each accepted remote connection is bidirectionally proxied to localAddr.
// Returns a stop function that cancels the forward and closes the remote listener.
// The stop function waits for all in-flight proxy goroutines to finish before returning.
func (c *Client) ReverseForward(remoteAddr, localAddr string) (func(), error) {
	ln, err := c.conn.Listen("tcp", remoteAddr)
	if err != nil {
		return nil, fmt.Errorf("remote Listen %s: %w", remoteAddr, err)
	}

	done := make(chan struct{})
	var wg sync.WaitGroup
	var once sync.Once

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			rConn, err := ln.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func(remote net.Conn) {
				defer wg.Done()
				proxyConn(remote, localAddr, done)
			}(rConn)
		}
	}()

	stop := func() {
		once.Do(func() {
			close(done)
			_ = ln.Close()
		})
		wg.Wait()
	}
	return stop, nil
}

func proxyConn(remote net.Conn, localAddr string, done <-chan struct{}) {
	defer remote.Close()
	local, err := net.Dial("tcp", localAddr)
	if err != nil {
		return
	}
	defer local.Close()

	errc := make(chan error, 2)
	go func() { _, err := io.Copy(remote, local); errc <- err }()
	go func() { _, err := io.Copy(local, remote); errc <- err }()

	select {
	case <-done:
	case <-errc:
	}
}

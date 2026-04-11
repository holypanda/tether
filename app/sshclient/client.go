package sshclient

import (
	"fmt"
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

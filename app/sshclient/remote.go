package sshclient

import (
	"bytes"
	"encoding/base64"
	"fmt"
)

// Run executes a single shell command on the remote host and returns its stdout.
// Exit status != 0 returns an error that includes stderr.
func (c *Client) Run(cmd string) ([]byte, error) {
	session, err := c.conn.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	if err := session.Run(cmd); err != nil {
		return stdout.Bytes(), fmt.Errorf("run %q: %w; stderr=%s", cmd, err, stderr.String())
	}
	return stdout.Bytes(), nil
}

// RunScript executes a multi-line bash script on the remote host by base64-encoding
// it and piping through `base64 -d | bash`. This avoids all quoting/escape issues.
func (c *Client) RunScript(script string) ([]byte, error) {
	encoded := base64.StdEncoding.EncodeToString([]byte(script))
	return c.Run(fmt.Sprintf("echo %s | base64 -d | bash", encoded))
}

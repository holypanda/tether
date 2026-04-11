package sshclient

import (
	"fmt"
	"strings"
)

// Bootstrap performs first-time VPS setup over an already-established SSH connection:
// (1) installs sshfs if missing and (2) registers adminPubKey in ~/.ssh/authorized_keys.
// Idempotent — safe to call multiple times. Requires that c is connected via password
// auth (since the admin key is not yet trusted by the VPS).
func (c *Client) Bootstrap(adminPubKey string) error {
	pub := strings.TrimSpace(adminPubKey)
	if !strings.HasPrefix(pub, "ssh-ed25519 ") {
		return fmt.Errorf("invalid admin pubkey")
	}
	script := fmt.Sprintf(`
set -e
mkdir -p ~/.ssh && chmod 700 ~/.ssh
touch ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys
grep -qxF %q ~/.ssh/authorized_keys || echo %q >> ~/.ssh/authorized_keys
if ! command -v sshfs >/dev/null 2>&1; then
  if command -v apt-get >/dev/null 2>&1; then
    apt-get update -qq >/dev/null 2>&1 || true
    DEBIAN_FRONTEND=noninteractive apt-get install -y sshfs >/dev/null
  elif command -v yum >/dev/null 2>&1; then
    yum install -y fuse-sshfs >/dev/null
  else
    echo "no package manager found" >&2
    exit 1
  fi
fi
command -v sshfs >/dev/null 2>&1 || { echo "sshfs install failed" >&2; exit 1; }
echo bootstrap-ok
`, pub, pub)
	out, err := c.RunScript(script)
	if err != nil {
		return fmt.Errorf("bootstrap failed: %w", err)
	}
	if !strings.Contains(string(out), "bootstrap-ok") {
		return fmt.Errorf("bootstrap output unexpected: %s", out)
	}
	return nil
}

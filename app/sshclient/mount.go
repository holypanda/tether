package sshclient

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

type MountParams struct {
	WinShareUser      string // e.g. "winshare" — identifier only, auth is by key
	RemoteTunnelPort  int    // port on VPS forwarded to exe's SFTP server
	RemoteMountPoint  string // e.g. ~/local-code
	SFTPPrivateKeyPEM []byte // ephemeral private key in OpenSSH PEM format
}

// MountHandle identifies a live sshfs mount so Unmount can tear it down.
// The temp key file is deleted by Mount on success, so Unmount has nothing
// to clean up beyond the mount point.
type MountHandle struct {
	MountPoint string
}

// winShareUserRe limits the share username to a small safe charset to prevent
// it from being an injection vector when interpolated into shell commands.
var winShareUserRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// bashQuote wraps s in single quotes, escaping embedded single quotes using the
// standard '\'' idiom. The result is safe for use as a single positional argument
// in POSIX shell, regardless of whitespace, backticks, $, or other metacharacters.
func bashQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// Mount writes the given private key to a random /tmp path on the VPS, runs sshfs
// pointing at localhost:RemoteTunnelPort, and immediately deletes the temp key file
// on success (sshfs has already loaded it into memory). Returns a MountHandle needed
// for Unmount. If sshfs fails, the temp key is cleaned up via a shell EXIT trap.
//
// The VPS must already have sshfs installed (see Bootstrap).
func (c *Client) Mount(p MountParams) (*MountHandle, error) {
	if !winShareUserRe.MatchString(p.WinShareUser) {
		return nil, fmt.Errorf("invalid WinShareUser %q: must match [a-zA-Z0-9_-]+", p.WinShareUser)
	}
	if p.RemoteMountPoint == "" {
		return nil, fmt.Errorf("RemoteMountPoint is empty")
	}
	if len(p.SFTPPrivateKeyPEM) == 0 {
		return nil, fmt.Errorf("SFTPPrivateKeyPEM is empty")
	}
	if p.RemoteTunnelPort <= 0 || p.RemoteTunnelPort > 65535 {
		return nil, fmt.Errorf("invalid RemoteTunnelPort %d", p.RemoteTunnelPort)
	}

	randBytes := make([]byte, 8)
	if _, err := rand.Read(randBytes); err != nil {
		return nil, err
	}
	// tempKey contains only hex chars: no shell escaping needed for this literal,
	// but we still quote it below for defense-in-depth.
	tempKey := fmt.Sprintf("/tmp/.stim-link-%s", hex.EncodeToString(randBytes))

	// Embed the PEM via base64 to avoid any heredoc boundary concerns.
	pemB64 := base64.StdEncoding.EncodeToString(p.SFTPPrivateKeyPEM)

	script := fmt.Sprintf(`
set -e
umask 077
trap 'rm -f %s' EXIT
echo %s | base64 -d > %s
chmod 600 %s
mkdir -p %s
fusermount -u %s 2>/dev/null || true
sshfs -p %d \
  -o IdentityFile=%s \
  -o StrictHostKeyChecking=no \
  -o UserKnownHostsFile=/dev/null \
  -o reconnect,ServerAliveInterval=15,ServerAliveCountMax=3 \
  -o cache=yes,compression=no \
  %s@localhost:/ %s
# sshfs succeeded; key is loaded in memory, safe to delete. Trap still covers
# any later failure before echo mount-ok.
rm -f %s
trap - EXIT
echo mount-ok
`,
		bashQuote(tempKey),            // trap path
		bashQuote(pemB64),             // echo arg (base64 is already URL-safe but quote anyway)
		bashQuote(tempKey),            // > redirection target
		bashQuote(tempKey),            // chmod arg
		bashQuote(p.RemoteMountPoint), // mkdir -p
		bashQuote(p.RemoteMountPoint), // fusermount -u
		p.RemoteTunnelPort,
		bashQuote(tempKey),            // IdentityFile
		bashQuote(p.WinShareUser),     // user@localhost
		bashQuote(p.RemoteMountPoint), // mount point
		bashQuote(tempKey),            // rm -f after success
	)

	out, err := c.RunScript(script)
	if err != nil {
		return nil, fmt.Errorf("mount failed: %w (output: %s)", err, out)
	}
	if !strings.Contains(string(out), "mount-ok") {
		return nil, fmt.Errorf("mount unexpected output: %s", out)
	}
	return &MountHandle{MountPoint: p.RemoteMountPoint}, nil
}

// Unmount reverses Mount via fusermount -u. Safe to call with a nil handle (no-op).
// The temp key file was already deleted by Mount on success; there's nothing else
// to clean up here.
func (c *Client) Unmount(h *MountHandle) error {
	if h == nil {
		return nil
	}
	script := fmt.Sprintf(`
set -e
fusermount -u %s 2>/dev/null || true
echo unmount-ok
`, bashQuote(h.MountPoint))
	out, err := c.RunScript(script)
	if err != nil {
		return fmt.Errorf("unmount failed: %w (output: %s)", err, out)
	}
	if !strings.Contains(string(out), "unmount-ok") {
		return fmt.Errorf("unmount unexpected output: %s", out)
	}
	return nil
}

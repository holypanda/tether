package sshclient

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

type MountParams struct {
	WinShareUser      string // fixed "winshare" identifier; auth is by key
	RemoteTunnelPort  int    // port on VPS forwarded to exe's SFTP server
	RemoteMountPoint  string // e.g. ~/local-code
	SFTPPrivateKeyPEM []byte // ephemeral private key in OpenSSH PEM format
}

type MountHandle struct {
	TempKeyPath string
	MountPoint  string
}

// Mount writes the given private key to a random /tmp path on the VPS and runs
// sshfs to mount localhost:RemoteTunnelPort as RemoteMountPoint. Returns a handle
// needed for Unmount. The VPS must already have sshfs installed (see Bootstrap).
func (c *Client) Mount(p MountParams) (*MountHandle, error) {
	randBytes := make([]byte, 8)
	if _, err := rand.Read(randBytes); err != nil {
		return nil, err
	}
	tempKey := fmt.Sprintf("/tmp/.stim-link-%s", hex.EncodeToString(randBytes))

	script := fmt.Sprintf(`
set -e
umask 077
cat > %s <<'KEOF'
%s
KEOF
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
echo mount-ok
`,
		tempKey, strings.TrimRight(string(p.SFTPPrivateKeyPEM), "\n"),
		tempKey,
		p.RemoteMountPoint, p.RemoteMountPoint,
		p.RemoteTunnelPort, tempKey,
		p.WinShareUser, p.RemoteMountPoint,
	)

	out, err := c.RunScript(script)
	if err != nil {
		return nil, fmt.Errorf("mount failed: %w (output: %s)", err, out)
	}
	if !strings.Contains(string(out), "mount-ok") {
		return nil, fmt.Errorf("mount unexpected output: %s", out)
	}
	return &MountHandle{TempKeyPath: tempKey, MountPoint: p.RemoteMountPoint}, nil
}

// Unmount reverses Mount: fusermount -u and delete the temp key file.
// Safe to call with a nil handle (no-op).
func (c *Client) Unmount(h *MountHandle) error {
	if h == nil {
		return nil
	}
	script := fmt.Sprintf(`
fusermount -u %s 2>/dev/null || true
rm -f %s
echo unmount-ok
`, h.MountPoint, h.TempKeyPath)
	out, err := c.RunScript(script)
	if err != nil {
		return fmt.Errorf("unmount failed: %w (output: %s)", err, out)
	}
	if !strings.Contains(string(out), "unmount-ok") {
		return fmt.Errorf("unmount unexpected output: %s", out)
	}
	return nil
}

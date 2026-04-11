//go:build windows

package identity

import (
	"fmt"
	"os/exec"
	"os/user"
)

// lockdownKeyFile sets restrictive NTFS ACLs on path: inheritance disabled,
// only the current user has full control. This is required on Windows because
// OpenSSH ssh.exe refuses to use private keys that are readable by anyone
// beyond the current user — it falls back to password prompt silently
// otherwise.
func lockdownKeyFile(path string) error {
	u, err := user.Current()
	if err != nil {
		return fmt.Errorf("user.Current: %w", err)
	}
	sidArg := "*" + u.Uid + ":F"
	args := []string{path, "/inheritance:r", "/grant:r", sidArg}
	if out, err := exec.Command("icacls", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("icacls %v: %w (%s)", args, err, out)
	}
	return nil
}

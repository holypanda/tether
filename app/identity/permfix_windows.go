//go:build windows

package identity

import (
	"fmt"
	"os/exec"
	"os/user"
	"syscall"
)

// createNoWindow is the Windows CREATE_NO_WINDOW flag. Setting it on
// SysProcAttr.CreationFlags prevents icacls (a console program) from opening
// a visible console window when spawned by a GUI-mode Go process — otherwise
// tether.exe appears to "flash" a cmd window on every connect.
const createNoWindow = 0x08000000

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
	cmd := exec.Command("icacls", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("icacls %v: %w (%s)", args, err, out)
	}
	return nil
}

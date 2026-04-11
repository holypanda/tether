//go:build windows

package claude

import (
	"fmt"
	"os/exec"
	"syscall"
)

type LaunchParams struct {
	AdminKeyPath     string
	VpsHost          string
	VpsUser          string
	VpsPort          int
	RemoteMountPoint string
}

// Launch opens a new PowerShell window that SSHes into the VPS and runs claude
// in the mount directory. The spawned process is independent — closing the
// Claude window does not affect the main exe's connection state.
func Launch(p LaunchParams) error {
	sshArgs := fmt.Sprintf(
		`ssh -i "%s" -p %d -o StrictHostKeyChecking=no -t %s@%s "cd %s && claude"`,
		p.AdminKeyPath, p.VpsPort, p.VpsUser, p.VpsHost, p.RemoteMountPoint,
	)
	psCmd := fmt.Sprintf(`Start-Process powershell -ArgumentList '-NoExit','-Command','%s'`, sshArgs)
	cmd := exec.Command("powershell.exe", "-NoProfile", "-Command", psCmd)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}

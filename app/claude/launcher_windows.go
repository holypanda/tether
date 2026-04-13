//go:build windows

package claude

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

type LaunchParams struct {
	AdminKeyPath     string
	VpsHost          string
	VpsUser          string
	VpsPort          int
	RemoteMountPoint string
}

// Launch opens a new console window with an interactive SSH session to the VPS.
// It uses the admin key that was set up during bootstrap, so no password prompt.
// The mount directory is passed so the session opens in ~/local-code by default.
func Launch(p LaunchParams) error {
	batchPath := filepath.Join(os.TempDir(), "tether-ssh.cmd")
	body := fmt.Sprintf("@echo off\r\n"+
		"title tether VPS shell (%s@%s)\r\n"+
		"ssh -i \"%s\" -p %d -o StrictHostKeyChecking=no -t %s@%s \"cd %s && exec bash\"\r\n"+
		"echo.\r\n"+
		"echo [SSH session ended - press any key to close]\r\n"+
		"pause >nul\r\n",
		p.VpsUser, p.VpsHost,
		p.AdminKeyPath, p.VpsPort, p.VpsUser, p.VpsHost, p.RemoteMountPoint,
	)
	if err := os.WriteFile(batchPath, []byte(body), 0o644); err != nil {
		return fmt.Errorf("write launch batch: %w", err)
	}

	cmd := exec.Command("cmd.exe", "/c", "start", "", "cmd.exe", "/k", batchPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}

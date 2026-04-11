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

// Launch opens a new console window running ssh → claude on the VPS.
//
// Implementation: write a temporary .cmd batch file and spawn it via
// `cmd.exe /c start`. We deliberately avoid PowerShell because PowerShell 5.x
// (the default shell on Windows 10) cannot parse the bash `&&` operator even
// when it's inside double quotes — it errors out with "not a valid statement
// separator" before the string ever reaches ssh.exe.
//
// The .cmd approach keeps the `&&` inside quotes where cmd.exe passes it through
// verbatim as part of ssh's argument, and the remote bash on the VPS handles it
// correctly.
func Launch(p LaunchParams) error {
	batchPath := filepath.Join(os.TempDir(), "stim-link-claude.cmd")
	body := fmt.Sprintf("@echo off\r\n"+
		"title stim-link Claude session\r\n"+
		"ssh -i \"%s\" -p %d -o StrictHostKeyChecking=no -t %s@%s \"cd %s && claude\"\r\n"+
		"echo.\r\n"+
		"echo [Claude session ended - press any key to close]\r\n"+
		"pause >nul\r\n",
		p.AdminKeyPath, p.VpsPort, p.VpsUser, p.VpsHost, p.RemoteMountPoint,
	)
	if err := os.WriteFile(batchPath, []byte(body), 0o644); err != nil {
		return fmt.Errorf("write launch batch: %w", err)
	}

	// `start "" cmd /k <batch>` opens a new console window that runs the batch
	// and stays open afterward (the batch's `pause` handles the final wait, but
	// `/k` guarantees the window sticks around even if the batch errors early).
	cmd := exec.Command("cmd.exe", "/c", "start", "", "cmd.exe", "/k", batchPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}

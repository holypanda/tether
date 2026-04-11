//go:build !windows

package claude

import "errors"

type LaunchParams struct {
	AdminKeyPath     string
	VpsHost          string
	VpsUser          string
	VpsPort          int
	RemoteMountPoint string
}

func Launch(p LaunchParams) error {
	return errors.New("claude launcher only supported on windows")
}

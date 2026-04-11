//go:build !windows

package identity

// lockdownKeyFile is a no-op on POSIX: os.WriteFile already honors the 0o600
// mode passed in generateAndSave.
func lockdownKeyFile(path string) error {
	return nil
}

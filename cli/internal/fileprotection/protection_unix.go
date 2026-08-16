//go:build !windows

package fileprotection

import (
	"fmt"
	"os"
)

// ProtectFile limits a sensitive file to its POSIX owner.
func ProtectFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("sensitive file is not a regular file")
	}
	return os.Chmod(path, 0o600)
}

// ProtectDirectory limits a sensitive directory to its POSIX owner.
func ProtectDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("sensitive directory is not a directory")
	}
	return os.Chmod(path, 0o700)
}

func replaceFile(source, destination string) error {
	return os.Rename(source, destination)
}

// ValidateDirectory verifies the platform-specific sensitive-directory policy.
func ValidateDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("sensitive directory is not a directory")
	}
	if got := info.Mode().Perm(); got != 0o700 {
		return fmt.Errorf("sensitive directory mode is %04o, want 0700", got)
	}
	return nil
}

// ValidateFile verifies the platform-specific sensitive-file policy.
func ValidateFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("sensitive file is not a regular file")
	}
	if got := info.Mode().Perm(); got != 0o600 {
		return fmt.Errorf("sensitive file mode is %04o, want 0600", got)
	}
	return nil
}

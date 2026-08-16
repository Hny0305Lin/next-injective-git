package fileprotection

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteFile writes sensitive data only after its staging file is protected,
// then replaces the destination without following an existing link.
func WriteFile(path string, data []byte) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, "."+filepath.Base(path)+"-staging-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	closed := false
	published := false
	defer func() {
		if !closed {
			_ = temporary.Close()
		}
		if !published {
			_ = os.Remove(temporaryPath)
		}
	}()

	if err := ProtectFile(temporaryPath); err != nil {
		return fmt.Errorf("protect sensitive-file staging file: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	closed = true
	if err := replaceFile(temporaryPath, path); err != nil {
		return fmt.Errorf("publish protected file: %w", err)
	}
	published = true
	return nil
}

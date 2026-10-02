package secrets

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// WriteFile writes data to a new owner-only file at path, which must be absolute.
// It never overwrites or follows an existing file or symlink.
func WriteFile(path string, data []byte) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("path %q must be absolute", path)
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("file %q already exists, choose a new path: %w", path, fs.ErrExist)
		}
		return fmt.Errorf("failed to create file %q: %w", path, err)
	}

	if _, err := f.Write(data); err != nil {
		_ = f.Close()       // the write error is the one worth reporting
		_ = os.Remove(path) // best effort: never leave a partial value behind
		return fmt.Errorf("failed to write file %q: %w", path, err)
	}

	if err := f.Close(); err != nil {
		_ = os.Remove(path) // best effort: never leave a partial value behind
		return fmt.Errorf("failed to close file %q: %w", path, err)
	}

	return nil
}

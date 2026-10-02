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

// WriteTempFile writes data to a file named name inside a new owner-only
// folder in the system temp directory, and returns the file's path.
func WriteTempFile(name string, data []byte) (string, error) {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
		name = "value"
	}

	dir, err := os.MkdirTemp("", "mcp-kubernetes-ro-secret-")
	if err != nil {
		return "", fmt.Errorf("failed to create temp folder: %w", err)
	}

	path := filepath.Join(dir, name)
	if err := WriteFile(path, data); err != nil {
		_ = os.RemoveAll(dir) // best effort: the folder is new and holds nothing else
		return "", err
	}

	return path, nil
}

package secrets

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// WriteFile writes data to a new file at path that only its owner can read.
// The path must be absolute so it does not depend on the server's working
// directory. It never overwrites or follows an existing file or symlink.
func WriteFile(path string, data []byte) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("path %q must be absolute", path)
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("%q already exists: choose a new path, existing files are never overwritten", path)
		}
		return fmt.Errorf("creating file: %w", err)
	}

	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(path)
		return fmt.Errorf("writing file: %w", err)
	}

	if err := f.Close(); err != nil {
		os.Remove(path)
		return fmt.Errorf("closing file: %w", err)
	}

	return nil
}

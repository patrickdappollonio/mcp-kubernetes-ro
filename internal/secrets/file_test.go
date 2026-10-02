package secrets

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// prepare returns the path to write to, creating whatever already
		// exists there.
		prepare  func(t *testing.T, dir string) string
		wantErr  error  // matched with errors.Is
		wantText string // a fact the error must state, for errors with no sentinel
	}{
		{
			name:    "creates a new owner-only file",
			prepare: func(_ *testing.T, dir string) string { return filepath.Join(dir, "db-password") },
		},
		{
			name: "refuses an existing file",
			prepare: func(t *testing.T, dir string) string {
				path := filepath.Join(dir, "existing")
				if err := os.WriteFile(path, []byte("keep me"), 0o600); err != nil {
					t.Fatal(err)
				}
				return path
			},
			wantErr: fs.ErrExist,
		},
		{
			name: "refuses a symlink",
			prepare: func(t *testing.T, dir string) string {
				link := filepath.Join(dir, "link")
				if err := os.Symlink(filepath.Join(dir, "target"), link); err != nil {
					t.Fatal(err)
				}
				return link
			},
			wantErr: fs.ErrExist,
		},
		{
			name:     "requires an absolute path",
			prepare:  func(*testing.T, string) string { return "relative/secret" },
			wantText: "must be absolute",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			path := tt.prepare(t, dir)
			before, _ := os.ReadFile(path)

			err := WriteFile(path, []byte("s3cr3t"))

			switch {
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("WriteFile(%q) error = %v, want %v", path, err, tt.wantErr)
				}
			case tt.wantText != "":
				if err == nil || !strings.Contains(err.Error(), tt.wantText) {
					t.Fatalf("WriteFile(%q) error = %v, want one stating %q", path, err, tt.wantText)
				}
			default:
				if err != nil {
					t.Fatalf("WriteFile(%q) returned unexpected error: %v", path, err)
				}
			}

			if err != nil {
				if after, _ := os.ReadFile(path); string(after) != string(before) {
					t.Errorf("WriteFile(%q) changed the existing file to %q", path, after)
				}
				if _, statErr := os.Stat(filepath.Join(dir, "target")); !errors.Is(statErr, fs.ErrNotExist) {
					t.Errorf("WriteFile(%q) created the symlink target (stat error: %v)", path, statErr)
				}
				return
			}

			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != "s3cr3t" {
				t.Errorf("file contents = %q, want %q", got, "s3cr3t")
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if perm := info.Mode().Perm(); perm != 0o600 {
				t.Errorf("file permissions = %o, want 600", perm)
			}
		})
	}
}

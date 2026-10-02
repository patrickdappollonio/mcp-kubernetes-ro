package secrets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFileCreatesOwnerOnlyFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "db-password")

	if err := WriteFile(path, []byte("s3cr3t")); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "s3cr3t" {
		t.Fatalf("file contents = %q, want %q", got, "s3cr3t")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("permissions = %o, want 600", perm)
	}
}

func TestWriteFileRefusesExistingFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "existing")
	if err := os.WriteFile(path, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := WriteFile(path, []byte("new"))
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("WriteFile() error = %v, want an 'already exists' error", err)
	}

	if got, _ := os.ReadFile(path); string(got) != "keep me" {
		t.Fatalf("existing file was changed to %q", got)
	}
}

func TestWriteFileRefusesSymlink(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	if err := WriteFile(link, []byte("value")); err == nil {
		t.Fatal("WriteFile() followed a symlink")
	}

	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("symlink target was created (stat error: %v)", err)
	}
}

func TestWriteFileRequiresAbsolutePath(t *testing.T) {
	t.Parallel()

	err := WriteFile("relative/secret", []byte("value"))
	if err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("WriteFile() error = %v, want an 'absolute' error", err)
	}
}

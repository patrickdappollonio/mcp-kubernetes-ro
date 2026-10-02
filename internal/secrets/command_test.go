package secrets

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellQuote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in, want string
	}{
		{"/tmp/key.pem", "'/tmp/key.pem'"},
		{"/tmp/my key.pem", "'/tmp/my key.pem'"},
		{"/tmp/it's.pem", `'/tmp/it'\''s.pem'`},
		{"$(rm -rf ~)", "'$(rm -rf ~)'"},
	}

	for _, tt := range tests {
		if got := shellQuote(tt.in); got != tt.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestDecryptCommandIncludesEveryBlockOnItsOwnLine(t *testing.T) {
	t.Parallel()

	cmd := DecryptCommand([]string{"AAAA", "BBBB"}, "/tmp/k.pem", "/tmp/out")

	if !strings.Contains(cmd, "\nAAAA\nBBBB\n") {
		t.Fatalf("command does not contain the blocks one per line:\n%s", cmd)
	}
}

// requireOpenSSL skips the test when the openssl CLI is not installed, since
// these tests exercise the exact command an agent runs on the user's machine.
func requireOpenSSL(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip("openssl not found in PATH")
	}
}

// generateKeyWithOpenSSL creates a key pair the same way the agent is told to
// and returns the private key path and the public key PEM.
func generateKeyWithOpenSSL(t *testing.T, dir string) (string, string) {
	t.Helper()
	keyPath := filepath.Join(dir, "key.pem")
	if out, err := exec.Command("openssl", "genrsa", "-out", keyPath, "2048").CombinedOutput(); err != nil {
		t.Fatalf("openssl genrsa: %v\n%s", err, out)
	}
	pub, err := exec.Command("openssl", "rsa", "-in", keyPath, "-pubout").Output()
	if err != nil {
		t.Fatalf("openssl rsa -pubout: %v", err)
	}
	return keyPath, string(pub)
}

func runShell(t *testing.T, script string) ([]byte, error) {
	t.Helper()
	return exec.Command("sh", "-c", script).CombinedOutput()
}

func TestDecryptCommandWithOpenSSL(t *testing.T) {
	t.Parallel()
	requireOpenSSL(t)

	tests := []struct {
		name  string
		value []byte
	}{
		{"short value", []byte("s3cr3t-p@ss\n")},
		{"multi-block value", bytes.Repeat([]byte("-----certificate line-----\n"), 100)},
		{"empty value", []byte{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			keyPath, pub := generateKeyWithOpenSSL(t, dir)
			outPath := filepath.Join(dir, "secret value")

			blocks, err := Encrypt(pub, tt.value)
			if err != nil {
				t.Fatalf("Encrypt() error = %v", err)
			}

			if out, err := runShell(t, DecryptCommand(blocks, keyPath, outPath)); err != nil {
				t.Fatalf("decrypt command failed: %v\n%s", err, out)
			}

			got, err := os.ReadFile(outPath)
			if err != nil {
				t.Fatalf("reading output: %v", err)
			}
			if !bytes.Equal(got, tt.value) {
				t.Fatalf("decrypted value mismatch: got %q, want %q", got, tt.value)
			}

			info, err := os.Stat(outPath)
			if err != nil {
				t.Fatal(err)
			}
			if perm := info.Mode().Perm(); perm != 0o600 {
				t.Fatalf("output permissions = %o, want 600", perm)
			}

			if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
				t.Fatalf("private key still exists after decryption (stat error: %v)", err)
			}
		})
	}
}

func TestDecryptCommandRefusesToOverwrite(t *testing.T) {
	t.Parallel()
	requireOpenSSL(t)

	dir := t.TempDir()
	keyPath, pub := generateKeyWithOpenSSL(t, dir)
	outPath := filepath.Join(dir, "existing")
	if err := os.WriteFile(outPath, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}

	blocks, err := Encrypt(pub, []byte("new value"))
	if err != nil {
		t.Fatal(err)
	}

	if out, err := runShell(t, DecryptCommand(blocks, keyPath, outPath)); err == nil {
		t.Fatalf("decrypt command succeeded over an existing file:\n%s", out)
	}

	if got, _ := os.ReadFile(outPath); string(got) != "keep me" {
		t.Fatalf("existing file was changed to %q", got)
	}
}

func TestDecryptCommandFailsOnDamagedBlock(t *testing.T) {
	t.Parallel()
	requireOpenSSL(t)

	dir := t.TempDir()
	keyPath, pub := generateKeyWithOpenSSL(t, dir)
	outPath := filepath.Join(dir, "out")

	blocks, err := Encrypt(pub, []byte("value"))
	if err != nil {
		t.Fatal(err)
	}
	damaged := []byte(blocks[0])
	damaged[10] = map[bool]byte{true: 'B', false: 'A'}[damaged[10] == 'A']

	if out, err := runShell(t, DecryptCommand([]string{string(damaged)}, keyPath, outPath)); err == nil {
		t.Fatalf("decrypt command succeeded with a damaged block:\n%s", out)
	}

	if _, err := os.Stat(outPath); !os.IsNotExist(err) {
		t.Fatalf("output file left behind after a failed decryption (stat error: %v)", err)
	}
}

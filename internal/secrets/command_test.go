package secrets

import (
	"bytes"
	"errors"
	"io/fs"
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
		name     string
		value    []byte
		existing string // contents of a file already at the output path
		damage   bool   // flip one character of the first block
		wantErr  bool
	}{
		{name: "short value", value: []byte("s3cr3t-p@ss\n")},
		{name: "multi-block value", value: bytes.Repeat([]byte("-----certificate line-----\n"), 100)},
		{name: "empty value", value: []byte{}},
		{name: "refuses to overwrite an existing file", value: []byte("new value"), existing: "keep me", wantErr: true},
		{name: "fails on a damaged block", value: []byte("value"), damage: true, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			keyPath, pub := generateKeyWithOpenSSL(t, dir)
			outPath := filepath.Join(dir, "secret value")
			if tt.existing != "" {
				if err := os.WriteFile(outPath, []byte(tt.existing), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			blocks, err := Encrypt(pub, tt.value)
			if err != nil {
				t.Fatalf("Encrypt() error = %v", err)
			}
			if tt.damage {
				damaged := []byte(blocks[0])
				damaged[10] = map[bool]byte{true: 'B', false: 'A'}[damaged[10] == 'A']
				blocks[0] = string(damaged)
			}

			out, err := runShell(t, DecryptCommand(blocks, keyPath, outPath))

			if tt.wantErr {
				if err == nil {
					t.Fatalf("decrypt command succeeded, want a failure:\n%s", out)
				}
				got, readErr := os.ReadFile(outPath)
				switch {
				case tt.existing != "" && string(got) != tt.existing:
					t.Errorf("existing file changed to %q, want %q", got, tt.existing)
				case tt.existing == "" && !errors.Is(readErr, fs.ErrNotExist):
					t.Errorf("output file left behind after a failed decryption (read error: %v)", readErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("decrypt command failed: %v\n%s", err, out)
			}

			got, err := os.ReadFile(outPath)
			if err != nil {
				t.Fatalf("reading output: %v", err)
			}
			if !bytes.Equal(got, tt.value) {
				t.Errorf("decrypted value = %q, want %q", got, tt.value)
			}
			info, err := os.Stat(outPath)
			if err != nil {
				t.Fatal(err)
			}
			if perm := info.Mode().Perm(); perm != 0o600 {
				t.Errorf("output permissions = %o, want 600", perm)
			}
			if _, err := os.Stat(keyPath); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("private key still exists after decryption (stat error: %v)", err)
			}
		})
	}
}
